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
	Name    string
	Address string
	Port    int
}

// ServiceRegistry handles the logic for registering a Polar service in Consul.
type ServiceRegistry struct {
	consul *api.Client
	logger hclog.Logger

	id        string
	name      string
	namespace string
	host      string
	ports     []NamedPort

	tries           uint64
	backoffInterval time.Duration
}

// NewServiceRegistry creates a new service registry instance
func NewServiceRegistry(logger hclog.Logger, consul *api.Client, service, namespace, host string, ports []NamedPort) *ServiceRegistry {
	return &ServiceRegistry{
		logger:          logger,
		consul:          consul,
		id:              uuid.New().String(),
		name:            service,
		namespace:       namespace,
		host:            host,
		ports:           ports,
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
	var checks api.AgentServiceChecks

	taggedAddresses := map[string]api.ServiceAddress{}
	for _, port := range s.ports {
		checks = append(checks, s.checkFor(port))
		taggedAddresses[port.Name] = api.ServiceAddress{
			Port:    port.Port,
			Address: port.Address,
		}
	}
	registration := &api.AgentServiceRegistration{
		Kind:            api.ServiceKind(api.IngressGateway),
		ID:              s.id,
		Name:            s.name,
		Address:         s.host,
		Checks:          checks,
		TaggedAddresses: taggedAddresses,
	}
	if s.namespace != "" && s.namespace != "default" {
		registration.Namespace = s.namespace
	}

	return s.consul.Agent().ServiceRegisterOpts(registration, (&api.ServiceRegisterOpts{}).WithContext(ctx))
}

func (s *ServiceRegistry) checkFor(port NamedPort) *api.AgentServiceCheck {
	return &api.AgentServiceCheck{
		Name:                           fmt.Sprintf("%s - %s", serviceCheckName, port.Name),
		TCP:                            fmt.Sprintf("%s:%d", s.host, port.Port),
		Interval:                       serviceCheckInterval,
		DeregisterCriticalServiceAfter: serviceDeregistrationTime,
	}
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
