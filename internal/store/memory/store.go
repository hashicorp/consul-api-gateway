package memory

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul-api-gateway/internal/store"
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
	routes   map[string]store.Route

	// This mutex acts as a stop-the-world type
	// global mutex, as the store is a singleton
	// what this means is that once a lock on the
	// mutex is acquired, any mutable operations
	// on the gateway interfaces wrapped by our
	// state-building structures can happen
	// concerns of thread-safety (unless they)
	// spin up additional goroutines.
	mutex sync.RWMutex

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
		routes:   make(map[string]store.Route),
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

func (s *Store) GetGateway(ctx context.Context, id core.GatewayID) (store.Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	gateway, found := s.gateways[id]
	if !found {
		return nil, nil
	}
	return gateway.Gateway, nil
}

func (s *Store) syncGateway(ctx context.Context, gateway *gatewayState) error {
	if tracker, ok := gateway.Gateway.(store.StatusTrackingGateway); ok {
		return tracker.TrackSync(ctx, func() (bool, error) {
			return gateway.Sync(ctx)
		})
	}
	_, err := gateway.Sync(ctx)
	return err
}

func (s *Store) syncGateways(ctx context.Context, gateways ...*gatewayState) error {
	var syncGroup multierror.Group

	for _, gw := range gateways {
		gateway := gw
		syncGroup.Go(func() error {
			return s.syncGateway(ctx, gateway)
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
		if tracker, ok := route.(store.StatusTrackingRoute); ok {
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

// care needs to be taken here, we spin up multiple goroutines to handle
// synchronization in parallel -- since we pass around some of the objects
// from our internal state to callbacks in our interfaces, it means we *must not*
// access any potentially references stored from previous callbacks in the
// status updating callbacks in our interfaces -- otherwise proper locking
// is needed.
func (s *Store) sync(ctx context.Context, gateways ...*gatewayState) error {
	var syncGroup multierror.Group

	if gateways == nil {
		for _, gateway := range s.gateways {
			gateways = append(gateways, gateway)
		}
	}

	syncGroup.Go(func() error {
		return s.syncGateways(ctx, gateways...)
	})
	syncGroup.Go(func() error {
		return s.syncRouteStatuses(ctx)
	})
	if err := syncGroup.Wait(); err != nil {
		return err
	}
	return nil
}

func (s *Store) Sync(ctx context.Context) error {
	return s.sync(ctx)
}

func (s *Store) DeleteRoute(ctx context.Context, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Trace("deleting route", "id", id)
	for _, gateway := range s.gateways {
		gateway.Remove(id)
	}

	delete(s.routes, id)

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func (s *Store) UpsertRoute(ctx context.Context, route store.Route) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := route.ID()

	switch compareRoutes(s.routes[id], route) {
	case store.CompareResultInvalid, store.CompareResultNewer:
		// we have an old or invalid route, ignore it
		return nil
	case store.CompareResultNotEqual:
		s.logger.Trace("detected route state change", "id", id)
		s.routes[id] = route

		// bind to gateways
		for _, gateway := range s.gateways {
			gateway.TryBind(route)
		}
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func compareRoutes(a, b store.Route) store.CompareResult {
	if b == nil {
		return store.CompareResultInvalid
	}
	if a == nil {
		return store.CompareResultNotEqual
	}
	return a.Compare(b)
}

func (s *Store) UpsertGateway(ctx context.Context, gateway store.Gateway) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := gateway.ID()

	current, found := s.gateways[id]

	switch current.Compare(gateway) {
	case store.CompareResultInvalid, store.CompareResultNewer:
		// we have an invalid or old route, ignore it
		return nil
	case store.CompareResultNotEqual:
		s.logger.Trace("detected gateway state change", "service", id.Service, "namespace", id.ConsulNamespace)
		updated := newGatewayState(s.logger, gateway, s.adapter)

		s.gateways[id] = updated

		// bind routes to this gateway
		for _, route := range s.routes {
			updated.TryBind(route)
		}

		if found && reflect.DeepEqual(current.Resolve(), updated.Resolve()) {
			// we have the exact same render tree, mark the gateway as already synced
			for _, listener := range updated.listeners {
				listener.MarkSynced()
			}
		}
	}

	if !found {
		// this was an insert
		activeGateways := atomic.AddInt64(&s.activeGateways, 1)
		metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))
	}

	// sync the gateway to consul and any updated route statuses
	return s.sync(ctx, s.gateways[id])
}

func (s *Store) DeleteGateway(ctx context.Context, id core.GatewayID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	gateway, found := s.gateways[id]
	if !found {
		return nil
	}

	s.logger.Trace("deleting gateway", "id", id)

	if err := s.adapter.Clear(ctx, id); err != nil {
		return err
	}
	for _, route := range s.routes {
		if tracker, ok := route.(store.StatusTrackingRoute); ok {
			tracker.OnGatewayRemoved(gateway)
		}
	}
	delete(s.gateways, id)

	activeGateways := atomic.AddInt64(&s.activeGateways, -1)
	metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))

	// sync route statuses to k8s
	return s.syncRouteStatuses(ctx)
}
