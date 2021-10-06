package memory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

var (
	ErrCannotBindListener = errors.New("cannot bind listener")
)

type Store struct {
	logger  hclog.Logger
	adapter core.SyncAdapter

	gateways map[core.GatewayID]*gatewayState
	routes   map[string]core.Route
	mutex    sync.RWMutex

	activeGateways int64
}

type StoreConfig struct {
	Adapter core.SyncAdapter
	Logger  hclog.Logger
}

func NewStore(config StoreConfig) *Store {
	return &Store{
		logger:   config.Logger,
		adapter:  config.Adapter,
		routes:   make(map[string]core.Route),
		gateways: make(map[core.GatewayID]*gatewayState),
	}
}

func (s *Store) GatewayExists(ctx context.Context, id core.GatewayID) (bool, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	_, found := s.gateways[id]
	return found, nil
}

func (s *Store) CanFetchSecrets(ctx context.Context, id core.GatewayID, secrets []string) (bool, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	gateway, found := s.gateways[id]
	if !found {
		return false, nil
	}
	for _, secret := range secrets {
		if _, found := gateway.secrets[secret]; !found {
			return false, nil
		}
	}
	return true, nil
}

func (s *Store) GetGateway(ctx context.Context, id core.GatewayID) (core.Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	gateway, found := s.gateways[id]
	if !found {
		return nil, nil
	}
	return gateway.Gateway, nil
}

func (s *Store) syncGateways(ctx context.Context) error {
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

func (s *Store) syncRouteStatuses(ctx context.Context) error {
	var syncGroup multierror.Group

	for _, r := range s.routes {
		route := r
		if tracker, ok := route.(core.StatusTrackingRoute); ok {
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

func (s *Store) Sync(ctx context.Context) error {
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

func (s *Store) GetRoute(ctx context.Context, id string) (core.Route, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	route, found := s.routes[id]
	if !found {
		return nil, nil
	}
	return route, nil
}

func (s *Store) DeleteRoute(ctx context.Context, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Trace("deleting route", "id", id)
	for _, gateway := range s.gateways {
		gateway.Remove(id)
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func (s *Store) UpsertRoute(ctx context.Context, route core.Route) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := route.ID()

	switch compareRoutes(s.routes[id], route) {
	case core.CompareResultInvalid, core.CompareResultNewer:
		// we have an old or invalid route, ignore it
		return nil
	case core.CompareResultNotEqual:
		s.logger.Trace("adding route", "id", id)

		s.routes[id] = route

		if initializable, ok := route.(core.InitializableRoute); ok {
			if err := initializable.Init(ctx); err != nil {
				// the route is considered invalid, so don't try to bind it at all
				return err
			}
		}

		// bind to gateways
		for _, gateway := range s.gateways {
			gateway.TryBind(route)
		}
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func compareRoutes(a, b core.Route) core.CompareResult {
	if b == nil {
		return core.CompareResultInvalid
	}
	if a == nil {
		return core.CompareResultNotEqual
	}
	return a.Compare(b)
}

func (s *Store) UpsertGateway(ctx context.Context, gateway core.Gateway) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := gateway.ID()

	current, found := s.gateways[id]
	switch current.Compare(gateway) {
	case core.CompareResultInvalid, core.CompareResultNewer:
		// we have an invalid or old route, ignore it
		return nil
	case core.CompareResultNotEqual:
		s.logger.Trace("adding gateway", "service", id.Service, "namespace", id.ConsulNamespace)

		updated := newGatewayState(s.logger, gateway, s.adapter)
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

func (s *Store) DeleteGateway(ctx context.Context, id core.GatewayID) error {
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
		if tracker, ok := route.(core.StatusTrackingRoute); ok {
			tracker.OnGatewayRemoved(gateway)
		}
	}
	delete(s.gateways, id)

	activeGateways := atomic.AddInt64(&s.activeGateways, -1)
	metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))

	// sync route statuses to k8s
	return s.syncRouteStatuses(ctx)
}

func (s *Store) State(ctx context.Context) ([]core.ResolvedGateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	resolved := []core.ResolvedGateway{}
	for _, gateway := range s.gateways {
		resolved = append(resolved, gateway.Resolve())
	}
	return resolved, nil
}
