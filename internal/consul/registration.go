// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/uuid"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

const (
	serviceCheckName          = "consul-api-gateway Gateway Listener"
	serviceCheckInterval      = time.Second * 10
	serviceCheckTTL           = time.Second * 20
	serviceDeregistrationTime = time.Minute
)

// ServiceRegistry handles the logic for registering a consul-api-gateway service in Consul.
// Note that the registry is *not* thread safe and should only ever call Register/Deregister
// from a single managing goroutine.
type ServiceRegistry struct {
	client Client
	logger hclog.Logger

	id        string
	name      string
	namespace string
	partition string
	host      string
	tags      []string

	cancel                 context.CancelFunc
	tries                  uint64
	backoffInterval        time.Duration
	reregistrationInterval time.Duration
	updateTTLInterval      time.Duration
}

// NewServiceRegistry creates a new service registry instance
func NewServiceRegistry(logger hclog.Logger, client Client, service, namespace, partition, host string) *ServiceRegistry {
	return &ServiceRegistry{
		logger:                 logger,
		client:                 client,
		id:                     uuid.New().String(),
		name:                   service,
		namespace:              namespace,
		partition:              partition,
		host:                   host,
		tries:                  defaultMaxAttempts,
		backoffInterval:        defaultBackoffInterval,
		reregistrationInterval: 30 * time.Second,
		updateTTLInterval:      10 * time.Second,
	}
}

// WithTags adds tags to associate with the service being registered.
func (s *ServiceRegistry) WithTags(tags []string) *ServiceRegistry {
	s.tags = tags
	return s
}

// WithTries tells the service registry to retry on any remote operations.
func (s *ServiceRegistry) WithTries(tries uint64) *ServiceRegistry {
	s.tries = tries
	return s
}

// Register registers a Gateway service with Consul.
func (s *ServiceRegistry) RegisterGateway(ctx context.Context, ttl bool) error {
	//serviceCheck := api.AgentServiceCheck{
	//	Name:                           fmt.Sprintf("%s - Ready", serviceCheckName),
	//	TCP:                            fmt.Sprintf("%s:%d", s.host, 20000),
	//	Interval:                       serviceCheckInterval,
	//	DeregisterCriticalServiceAfter: serviceDeregistrationTime,
	//}
	//if ttl {
	//	serviceCheck = api.AgentServiceCheck{
	//		CheckID:                        s.id,
	//		Name:                           fmt.Sprintf("%s - Health", s.name),
	//		TTL:                            serviceCheckTTL,
	//		DeregisterCriticalServiceAfter: serviceDeregistrationTime,
	//	}
	//}

	serviceChecks := api.HealthChecks{{
		Name: fmt.Sprintf("%s - Ready", serviceCheckName),
		Definition: api.HealthCheckDefinition{
			TCP:                                    fmt.Sprintf("%s:%d", s.host, 20000),
			IntervalDuration:                       serviceCheckInterval,
			DeregisterCriticalServiceAfterDuration: serviceDeregistrationTime,
		},
	}}
	if ttl {
		serviceChecks = api.HealthChecks{{
			CheckID: s.id,
			Name:    fmt.Sprintf("%s - Health", s.name),
			Definition: api.HealthCheckDefinition{
				TCP:                                    fmt.Sprintf("%s:%d", s.host, 20000),
				TimeoutDuration:                        serviceCheckTTL,
				DeregisterCriticalServiceAfterDuration: serviceDeregistrationTime,
			},
		}}
	}

	return s.register(ctx, &api.CatalogRegistration{
		Service: &api.AgentService{
			Kind:      api.ServiceKind(api.IngressGateway),
			ID:        s.id,
			Service:   s.name,
			Namespace: s.namespace,
			Partition: s.partition,
			Address:   s.host,
			Tags:      s.tags,
			Meta: map[string]string{
				"external-source": "consul-api-gateway",
			},
		},
		Checks: serviceChecks,
	}, ttl)
}

// Register registers a service with Consul.
func (s *ServiceRegistry) Register(ctx context.Context) error {
	return s.register(ctx, &api.CatalogRegistration{
		Service: &api.AgentService{
			Kind:      api.ServiceKindTypical,
			ID:        s.id,
			Service:   s.name,
			Namespace: s.namespace,
			Partition: s.partition,
			Address:   s.host,
			Tags:      s.tags,
		},

		Checks: api.HealthChecks{{
			CheckID: s.id,
			Name:    fmt.Sprintf("%s - Health", s.name),
			Definition: api.HealthCheckDefinition{
				TimeoutDuration:                        serviceCheckTTL,
				DeregisterCriticalServiceAfterDuration: serviceDeregistrationTime,
			},
		}},
	}, true)
}

func (s *ServiceRegistry) updateTTL(ctx context.Context) error {
	opts := &api.QueryOptions{}
	return s.client.Agent().UpdateTTLOpts(s.id, "service healthy", "pass", opts.WithContext(ctx))
}

func (s *ServiceRegistry) register(ctx context.Context, registration *api.CatalogRegistration, ttl bool) error {
	if s.cancel != nil {
		return nil
	}

	if err := s.retryRegistration(ctx, registration); err != nil {
		return err
	}
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go func() {
		for {
			select {
			case <-time.After(s.reregistrationInterval):
				s.ensureRegistration(childCtx, registration)
			case <-childCtx.Done():
				return
			}
		}
	}()

	if ttl {
		go func() {
			for {
				select {
				case <-time.After(s.updateTTLInterval):
					s.updateTTL(childCtx)
				case <-childCtx.Done():
					return
				}
			}
		}()
	}

	return nil
}

func (s *ServiceRegistry) ensureRegistration(ctx context.Context, registration *api.CatalogRegistration) {
	_, _, err := s.client.Agent().Service(s.id, &api.QueryOptions{
		Namespace: s.namespace,
	})
	if err == nil {
		return
	}

	var statusError api.StatusError
	if errors.As(err, &statusError) {
		if statusError.Code == http.StatusNotFound {
			if err := s.retryRegistration(ctx, registration); err != nil {
				s.logger.Error("error registering service", "error", err)
				return
			}
			s.logger.Info("successfully registered agent service")
			// early return here because we had no error
			// re-registering, so don't log
			return
		}
	}
	s.logger.Error("error fetching service", "error", err)
}

func (s *ServiceRegistry) retryRegistration(ctx context.Context, registration *api.CatalogRegistration) error {
	return backoff.Retry(func() error {
		err := s.registerService(ctx, registration)
		if err != nil {
			s.logger.Error("error registering service", "error", err)
		}
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(s.backoffInterval), s.tries), ctx))
}

func (s *ServiceRegistry) registerService(ctx context.Context, registration *api.CatalogRegistration) error {
	_, err := s.client.Catalog().Register(registration, nil)
	return err
	//return s.client.Agent().ServiceRegisterOpts(registration, (&api.ServiceRegisterOpts{}).WithContext(ctx))
}

// Deregister de-registers a service from Consul.
func (s *ServiceRegistry) Deregister(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	return backoff.Retry(func() error {
		err := s.deregister(ctx)
		if err != nil {
			s.logger.Error("error deregistering service", "error", err)
		}
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(s.backoffInterval), s.tries), ctx))
}

func (s *ServiceRegistry) deregister(ctx context.Context) error {
	return s.client.Agent().ServiceDeregisterOpts(s.id, (&api.QueryOptions{
		Namespace: s.namespace,
	}).WithContext(ctx))
}

func (s *ServiceRegistry) ID() string {
	return s.id
}

func (s *ServiceRegistry) Namespace() string {
	return s.namespace
}

func (s *ServiceRegistry) Partition() string {
	return s.partition
}
