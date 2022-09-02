package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

var (
	consulSyncInterval = 60 * time.Second
	startSyncLoopOnce  sync.Once
)

type Store struct {
	logger  hclog.Logger
	adapter core.SyncAdapter

	gateways map[core.GatewayID]store.Gateway
	routes   map[string]store.Route

	// This mutex acts as a stop-the-world type global mutex, as the store is a singleton.
	// What this means is that once a lock on the mutex is acquired, any mutable operations
	// on the gateway interfaces wrapped by our state-building structures can happen without
	// concerns of thread-safety (unless they) spin up additional goroutines.
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
		gateways: make(map[core.GatewayID]store.Gateway),
	}
}

func (s *Store) GatewayExists(ctx context.Context, id core.GatewayID) (bool, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	_, found := s.gateways[id]
	return found, nil
}

func (s *Store) GetGateway(ctx context.Context, id core.GatewayID) (store.Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	gateway, found := s.gateways[id]
	if !found {
		return nil, nil
	}
	return gateway, nil
}

func (s *Store) syncGateway(ctx context.Context, gateway store.Gateway) error {
	if tracker, ok := gateway.(store.StatusTrackingGateway); ok {
		return tracker.TrackSync(ctx, func() (bool, error) {
			return s.adapter.Sync(ctx, gateway.Resolve())
		})
	}
	_, err := s.adapter.Sync(ctx, gateway.Resolve())
	return err
}

func (s *Store) syncGateways(ctx context.Context, gateways ...store.Gateway) error {
	var syncGroup multierror.Group

	for _, gw := range gateways {
		gateway := gw
		syncGroup.Go(func() error {
			return s.syncGateway(ctx, gateway)
		})
	}
	if err := syncGroup.Wait().ErrorOrNil(); err != nil {
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
	if err := syncGroup.Wait().ErrorOrNil(); err != nil {
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
func (s *Store) sync(ctx context.Context, gateways ...store.Gateway) error {
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

	return syncGroup.Wait().ErrorOrNil()
}

func (s *Store) Sync(ctx context.Context) error {
	return s.sync(ctx)
}

// SyncAtInterval syncs the objects in the store w/ Consul at a constant interval
// until the provided context is cancelled. Calling SyncAtInterval multiple times
// will only result in a single sync loop as it should only be called during startup.
func (s *Store) SyncAtInterval(ctx context.Context) {
	startSyncLoopOnce.Do(func() {
		ticker := time.NewTicker(consulSyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mutex.Lock()

				if err := s.sync(ctx); err != nil {
					s.logger.Warn("An error occurred during memory store sync, some changes may be out of sync", "error", err)
				} else {
					s.logger.Trace("Synced memory store in background")
				}
				s.mutex.Unlock()
			}
		}
	})
}

func (s *Store) DeleteRoute(ctx context.Context, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Trace("deleting route", "id", id)
	for _, gateway := range s.gateways {
		gateway.Remove(ctx, id)
	}

	delete(s.routes, id)

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func (s *Store) UpsertRoute(ctx context.Context, route store.Route, updateConditionFn func(current store.Route) bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := route.ID()

	if updateConditionFn != nil && !updateConditionFn(s.routes[id]) {
		// No-op
		return nil
	}

	s.routes[id] = route

	// bind to gateways
	for _, gateway := range s.gateways {
		gateway.Bind(ctx, route)
	}

	// sync the gateways to consul and route statuses to k8s
	return s.Sync(ctx)
}

func (s *Store) UpsertGateway(ctx context.Context, gateway store.Gateway, updateConditionFn func(current store.Gateway) bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := gateway.ID()

	current, found := s.gateways[id]

	if updateConditionFn != nil && !updateConditionFn(current) {
		// No-op
		return nil
	}

	s.gateways[id] = gateway

	// bind routes to this gateway
	for _, route := range s.routes {
		gateway.Bind(ctx, route)
	}

	if !found {
		// this was an insert
		activeGateways := atomic.AddInt64(&s.activeGateways, 1)
		metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))
	}

	// sync the gateway to consul and any updated route statuses
	return s.sync(ctx, gateway)
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
