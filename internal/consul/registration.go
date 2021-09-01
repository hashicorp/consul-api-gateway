package consul

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

// ServiceRegistry handles the logic for registering a Polar service in Consul.
type ServiceRegistry struct {
	consul *api.Client
	logger hclog.Logger

	id        string
	name      string
	namespace string
	hostPort  string

	tries           uint64
	backoffInterval time.Duration
}

// NewServiceRegistry creates a new service registry instance
func NewServiceRegistry(logger hclog.Logger, consul *api.Client, service, namespace, hostPort string) *ServiceRegistry {
	return &ServiceRegistry{
		logger:          logger,
		consul:          consul,
		id:              uuid.New().String(),
		name:            service,
		namespace:       namespace,
		hostPort:        hostPort,
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
	tokens := strings.SplitN(s.hostPort, ":", 2)
	if len(tokens) != 2 {
		return fmt.Errorf("invalid port/ip pair: '%v'", s.hostPort)
	}
	port, err := strconv.Atoi(tokens[1])
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	registration := &api.AgentServiceRegistration{
		ID:      s.id,
		Name:    s.name,
		Port:    port,
		Address: tokens[0],
		Checks: api.AgentServiceChecks{{
			Name:                           serviceCheckName,
			TCP:                            s.hostPort,
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
