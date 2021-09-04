package consul

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/uuid"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

const (
	serviceCheckName          = "Polar Gateway Listener"
	serviceCheckInterval      = "10s"
	serviceDeregistrationTime = "1m"
)

// NamedPort is a tuple for ports with names
type NamedPort struct {
	Name     string
	Address  string
	Port     int
	Protocol string
}

// ServiceRegistry handles the logic for registering a Polar service in Consul.
type ServiceRegistry struct {
	consul *api.Client
	logger hclog.Logger

	id        string
	name      string
	namespace string
	host      string

	tries           uint64
	backoffInterval time.Duration
}

// NewServiceRegistry creates a new service registry instance
func NewServiceRegistry(logger hclog.Logger, consul *api.Client, service, namespace, host string) *ServiceRegistry {
	return &ServiceRegistry{
		logger:          logger,
		consul:          consul,
		id:              uuid.New().String(),
		name:            service,
		namespace:       namespace,
		host:            host,
		tries:           defaultMaxAttempts,
		backoffInterval: defaultBackoffInterval,
	}
}

// WithTries tells the service registry to retry on any remote operations.
func (s *ServiceRegistry) WithTries(tries uint64) *ServiceRegistry {
	s.tries = tries
	return s
}

// Register registers a service with Consul.
func (s *ServiceRegistry) Register(ctx context.Context) error {
	return backoff.Retry(func() error {
		err := s.register(ctx)
		if err != nil {
			s.logger.Error("error registering service", "error", err)
		}
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(s.backoffInterval), s.tries), ctx))
}

func (s *ServiceRegistry) register(ctx context.Context) error {
	registration := &api.AgentServiceRegistration{
		Kind:    api.ServiceKind(api.IngressGateway),
		ID:      s.id,
		Name:    s.name,
		Address: s.host,
		Checks: api.AgentServiceChecks{{
			Name:                           fmt.Sprintf("%s - Ready", serviceCheckName),
			TCP:                            fmt.Sprintf("%s:%d", s.host, 20000),
			Interval:                       serviceCheckInterval,
			DeregisterCriticalServiceAfter: serviceDeregistrationTime,
		}},
	}
	if s.namespace != "" && s.namespace != "default" {
		registration.Namespace = s.namespace
	}

	return s.consul.Agent().ServiceRegisterOpts(registration, (&api.ServiceRegisterOpts{}).WithContext(ctx))
}

// Deregister de-registers a service from Consul.
func (s *ServiceRegistry) Deregister(ctx context.Context) error {
	return backoff.Retry(func() error {
		err := s.deregister(ctx)
		if err != nil {
			s.logger.Error("error deregistering service", "error", err)
		}
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(s.backoffInterval), s.tries), ctx))
}

func (s *ServiceRegistry) deregister(ctx context.Context) error {
	return s.consul.Agent().ServiceDeregisterOpts(s.id, (&api.QueryOptions{}).WithContext(ctx))
}

func (s *ServiceRegistry) ID() string {
	return s.id
}
