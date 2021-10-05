package state

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

var (
	ErrCannotBindListener = errors.New("cannot bind listener")
)

type State struct {
	logger hclog.Logger
	consul *api.Client

	gateways map[GatewayID]*BoundGateway
	routes   map[string]Route
	mutex    sync.RWMutex

	activeGateways int64
}

type StateConfig struct {
	Consul *api.Client
	Logger hclog.Logger
}

func NewState(config StateConfig) *State {
	return &State{
		logger:   config.Logger,
		consul:   config.Consul,
		routes:   make(map[string]Route),
		gateways: make(map[GatewayID]*BoundGateway),
	}
}

// GatewayExists checks if the registry knows about a gateway
func (s *State) GatewayExists(id GatewayID) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	_, found := s.gateways[id]
	return found
}

// CanFetchSecrets checks if a gateway should be able to access a set of secrets
func (s *State) CanFetchSecrets(id GatewayID, secrets []string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	gateway, found := s.gateways[id]
	if !found {
		log.Println("foo not found")
		return false
	}
	for _, secret := range secrets {
		if _, found := gateway.secrets[secret]; !found {
			return false
		}
	}
	return true
}

func (s *State) syncGateways(ctx context.Context) error {
	var syncGroup multierror.Group

	for _, gw := range s.gateways {
		gateway := gw
		syncGroup.Go(func() error {
			return gateway.Sync(ctx)
		})
	}
	if err := syncGroup.Wait(); err != nil {
		s.logger.Error("error syncing gateways", "error", err)
		return err
	}
	return nil
}

func (s *State) syncRouteStatuses(ctx context.Context) error {
	var syncGroup multierror.Group

	for _, r := range s.routes {
		route := r
		if tracker, ok := route.(StatusTrackingRoute); ok {
			syncGroup.Go(func() error {
				return tracker.SyncStatus(ctx)
			})
		}
	}
	if err := syncGroup.Wait(); err != nil {
		s.logger.Error("error syncing route statuses", "error", err)
		return err
	}
	return nil
}

func (s *State) Sync(ctx context.Context) error {
	var syncGroup multierror.Group

	syncGroup.Go(func() error {
		return s.syncGateways(ctx)
	})
	syncGroup.Go(func() error {
		return s.syncRouteStatuses(ctx)
	})
	if err := syncGroup.Wait(); err != nil {
		return err
	}
	return nil
}

func (s *State) DeleteRoute(ctx context.Context, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Trace("deleting route", "id", id)
	for _, gateway := range s.gateways {
		gateway.Remove(id)
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func (s *State) AddRoute(ctx context.Context, route Route) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := route.ID()

	switch compareRoutes(s.routes[id], route) {
	case CompareResultInvalid, CompareResultNewer:
		// we have an old or invalid route, ignore it
		return nil
	case CompareResultNotEqual:
		s.logger.Trace("adding route", "id", id)

		s.routes[id] = route

		// resolve any service references for the route
		if err := route.ResolveServices(ctx); err != nil {
			// the route is considered invalid, so don't try to bind it at all
			return err
		}
		// bind to gateways
		for _, gateway := range s.gateways {
			gateway.TryBind(route)
		}
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func compareRoutes(a, b Route) CompareResult {
	if b == nil {
		return CompareResultInvalid
	}
	if a == nil {
		return CompareResultNotEqual
	}
	return a.Compare(b)
}

func (s *State) AddGateway(ctx context.Context, gateway Gateway) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := gateway.ID()

	current, found := s.gateways[id]
	switch current.Compare(gateway) {
	case CompareResultInvalid, CompareResultNewer:
		// we have an invalid or old route, ignore it
		return nil
	case CompareResultNotEqual:
		s.logger.Trace("adding gateway", "service", id.Service, "namespace", id.ConsulNamespace)

		updated := NewBoundGateway(gateway, s.consul)
		updated.Merge(current)
		if err := updated.ResolveListenerTLS(ctx); err != nil {
			// we have invalid listener references, consider the gateway bad
			return err
		}

		s.gateways[id] = updated

		// bind routes to this gateway
		for _, route := range s.routes {
			updated.TryBind(route)
		}
	}

	if !found {
		// this was an insert
		activeGateways := atomic.AddInt64(&s.activeGateways, 1)
		metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func (s *State) DeleteGateway(ctx context.Context, id GatewayID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	gateway, found := s.gateways[id]
	if !found {
		return nil
	}

	s.logger.Trace("deleting gateway", "id", id)

	// deregistration of the service in the gateway
	// handles resource cleanup, we can just remove
	// it from being tracked and sync back route statuses
	for _, route := range s.routes {
		if tracker, ok := route.(StatusTrackingRoute); ok {
			tracker.OnGatewayRemoved(gateway)
		}
	}
	delete(s.gateways, id)

	activeGateways := atomic.AddInt64(&s.activeGateways, -1)
	metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))

	// sync route statuses to k8s
	return s.syncRouteStatuses(ctx)
}
