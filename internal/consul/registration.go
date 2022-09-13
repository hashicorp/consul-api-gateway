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
	serviceCheckInterval      = "10s"
	serviceCheckTTL           = "20s"
	serviceDeregistrationTime = "1m"
)

// ServiceRegistry handles the logic for registering a consul-api-gateway service in Consul.
// Note that the registry is *not* thread safe and should only ever call Register/Deregister
// from a single managing goroutine.
type ServiceRegistry struct {
	consul *api.Client
	logger hclog.Logger

	id        string
	name      string
	namespace string
	host      string
	tags      []string

	cancel                 context.CancelFunc
	tries                  uint64
	backoffInterval        time.Duration
	reregistrationInterval time.Duration
	updateTTLInterval      time.Duration
}

// NewServiceRegistry creates a new service registry instance
func NewServiceRegistry(logger hclog.Logger, consul *api.Client, service, namespace, host string) *ServiceRegistry {
	return &ServiceRegistry{
		logger:                 logger,
		consul:                 consul,
		id:                     uuid.New().String(),
		name:                   service,
		namespace:              namespace,
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
func (s *ServiceRegistry) RegisterGateway(ctx context.Context) error {
	return s.register(ctx, &api.AgentServiceRegistration{
		Kind:      api.ServiceKind(api.IngressGateway),
		ID:        s.id,
		Name:      s.name,
		Namespace: s.namespace,
		Address:   s.host,
		Tags:      s.tags,
		Meta: map[string]string{
			"external-source": "consul-api-gateway",
		},
		Checks: api.AgentServiceChecks{{
			Name:                           fmt.Sprintf("%s - Ready", serviceCheckName),
			TCP:                            fmt.Sprintf("%s:%d", s.host, 20000),
			Interval:                       serviceCheckInterval,
			DeregisterCriticalServiceAfter: serviceDeregistrationTime,
		}},
	}, false)
}

// Register registers a service with Consul.
func (s *ServiceRegistry) Register(ctx context.Context) error {
	return s.register(ctx, &api.AgentServiceRegistration{
		Kind:      api.ServiceKindTypical,
		ID:        s.id,
		Name:      s.name,
		Namespace: s.namespace,
		Address:   s.host,
		Tags:      s.tags,
		Checks: api.AgentServiceChecks{{
			CheckID:                        s.id,
			Name:                           fmt.Sprintf("%s - Health", s.name),
			TTL:                            serviceCheckTTL,
			DeregisterCriticalServiceAfter: serviceDeregistrationTime,
		}},
	}, true)
}

func (s *ServiceRegistry) updateTTL(ctx context.Context) error {
	opts := &api.QueryOptions{}
	return s.consul.Agent().UpdateTTLOpts(s.id, "service healthy", "pass", opts.WithContext(ctx))
}

func (s *ServiceRegistry) register(ctx context.Context, registration *api.AgentServiceRegistration, ttl bool) error {
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

func (s *ServiceRegistry) ensureRegistration(ctx context.Context, registration *api.AgentServiceRegistration) {
	_, _, err := s.consul.Agent().Service(s.id, &api.QueryOptions{
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

func (s *ServiceRegistry) retryRegistration(ctx context.Context, registration *api.AgentServiceRegistration) error {
	return backoff.Retry(func() error {
		err := s.registerService(ctx, registration)
		if err != nil {
			s.logger.Error("error registering service", "error", err)
		}
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(s.backoffInterval), s.tries), ctx))
}

func (s *ServiceRegistry) registerService(ctx context.Context, registration *api.AgentServiceRegistration) error {
	return s.consul.Agent().ServiceRegisterOpts(registration, (&api.ServiceRegisterOpts{}).WithContext(ctx))
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
	return s.consul.Agent().ServiceDeregisterOpts(s.id, (&api.QueryOptions{
		Namespace: s.namespace,
	}).WithContext(ctx))
}

func (s *ServiceRegistry) ID() string {
	return s.id
}

func (s *ServiceRegistry) Namespace() string {
	return s.namespace
}
