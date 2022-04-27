package store

import (
	"context"
	"errors"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

type store struct {
	logger        hclog.Logger
	adapter       core.SyncAdapter
	backend       Backend
	marshaler     Marshaler
	binder        Binder
	statusUpdater StatusUpdater

	// This mutex acts as a stop-the-world type
	// global mutex, as the store is a singleton
	// what this means is that once a lock on the
	// mutex is acquired, any mutable operations
	// on the gateway interfaces wrapped by our
	// state-building structures can happen
	// concerns of thread-safety (unless they)
	// spin up additional goroutines.
	mutex sync.RWMutex
}

var _ Store = &store{}

type Config struct {
	Logger        hclog.Logger
	Adapter       core.SyncAdapter
	Backend       Backend
	Marshaler     Marshaler
	StatusUpdater StatusUpdater
	Binder        Binder
}

func New(c Config) Store {
	return &store{
		logger:        c.Logger,
		adapter:       c.Adapter,
		backend:       c.Backend,
		marshaler:     c.Marshaler,
		binder:        c.Binder,
		statusUpdater: c.StatusUpdater,
	}
}

var (
	ErrNotFound           = errors.New("not found")
	ErrCannotBindListener = errors.New("cannot bind listener")
)

// CRUD for Gateways

func (s *store) GetGateway(ctx context.Context, id core.GatewayID) (Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.getGateway(ctx, id)
}

func (s *store) getGateway(ctx context.Context, id core.GatewayID) (Gateway, error) {
	data, err := s.backend.GetGateway(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return s.marshaler.UnmarshalGateway(data)
}

func (s *store) GetGateways(ctx context.Context) ([]Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.allGateways(ctx)
}

func (s *store) allGateways(ctx context.Context) ([]Gateway, error) {
	gateways, err := s.backend.GetGateways(ctx)
	if err != nil {
		return nil, err
	}
	unmarshaled := make([]Gateway, len(gateways))
	for i, data := range gateways {
		gateway, err := s.marshaler.UnmarshalGateway(data)
		if err != nil {
			return nil, err
		}
		unmarshaled[i] = gateway
	}
	return unmarshaled, nil
}

func (s *store) UpsertGateway(ctx context.Context, gateway Gateway, updateConditionFn func(current Gateway) bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	current, err := s.getGateway(ctx, gateway.ID())
	if err != nil {
		return err
	}

	if updateConditionFn != nil && !updateConditionFn(current) {
		// No-op
		return nil
	}

	routes, err := s.allRoutes(ctx)
	if err != nil {
		return err
	}

	_, boundRoutes, err := s.bindAll(ctx, []Gateway{gateway}, routes)
	if err != nil {
		return err
	}
	// ensure we always upsert the gateway, even if it didn't bind to any
	// routes
	gateways := []Gateway{gateway}
	if err := s.upsertAll(ctx, gateways, boundRoutes); err != nil {
		return err
	}

	if err := s.upsertAll(ctx, gateways, boundRoutes); err != nil {
		return err
	}

	// sync the gateways via adapter and route statuses
	return s.sync(ctx, gateways, boundRoutes)
}

func (s *store) DeleteGateway(ctx context.Context, id core.GatewayID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	gateway, err := s.getGateway(ctx, id)
	if err != nil {
		return err
	}
	if gateway == nil {
		return nil
	}

	s.logger.Trace("deleting gateway", "id", id)

	if err := s.adapter.Clear(ctx, id); err != nil {
		return err
	}

	routes, err := s.allRoutes(ctx)
	if err != nil {
		return err
	}

	_, unboundRoutes, err := s.unbindAll(ctx, []Gateway{gateway}, routes)
	if err != nil {
		return err
	}

	if err := s.backend.DeleteGateway(ctx, id); err != nil {
		return err
	}
	if err := s.upsertAll(ctx, nil, unboundRoutes); err != nil {
		return err
	}

	// sync the route statuses
	return s.syncRouteStatuses(ctx, unboundRoutes)
}

// CRUD for Routes

func (s *store) GetRoute(ctx context.Context, id string) (Route, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.getRoute(ctx, id)
}

func (s *store) getRoute(ctx context.Context, id string) (Route, error) {
	data, err := s.backend.GetRoute(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return s.marshaler.UnmarshalRoute(data)
}

func (s *store) GetRoutes(ctx context.Context) ([]Route, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.allRoutes(ctx)
}

func (s *store) allRoutes(ctx context.Context) ([]Route, error) {
	routes, err := s.backend.GetRoutes(ctx)
	if err != nil {
		return nil, err
	}
	unmarshaled := make([]Route, len(routes))
	for i, data := range routes {
		route, err := s.marshaler.UnmarshalRoute(data)
		if err != nil {
			return nil, err
		}
		unmarshaled[i] = route
	}
	return unmarshaled, nil
}

func (s *store) UpsertRoute(ctx context.Context, route Route, updateConditionFn func(current Route) bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	current, err := s.getRoute(ctx, route.ID())
	if err != nil {
		return err
	}

	if updateConditionFn != nil && !updateConditionFn(current) {
		// No-op
		return nil
	}

	gateways, err := s.allGateways(ctx)
	if err != nil {
		return err
	}

	boundGateways, _, err := s.bindAll(ctx, gateways, []Route{route})
	if err != nil {
		return err
	}
	// ensure we always upsert the route, even if it didn't bind to any
	// gateways
	routes := []Route{route}
	if err := s.upsertAll(ctx, boundGateways, routes); err != nil {
		return err
	}

	// sync the gateways via adapter and route statuses
	return s.sync(ctx, boundGateways, routes)
}

func (s *store) DeleteRoute(ctx context.Context, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	route, err := s.getRoute(ctx, id)
	if err != nil {
		return err
	}
	if route == nil {
		return nil
	}

	gateways, err := s.allGateways(ctx)
	if err != nil {
		return err
	}

	unboundGateways, _, err := s.unbindAll(ctx, gateways, []Route{route})
	if err != nil {
		return err
	}

	if err := s.backend.DeleteRoute(ctx, id); err != nil {
		return err
	}
	if err := s.upsertAll(ctx, unboundGateways, nil); err != nil {
		return err
	}

	// sync the gateways via adapter, no routes to sync
	return s.sync(ctx, unboundGateways, nil)
}

func bindOrUnbindAll(ctx context.Context, gateways []Gateway, routes []Route, f func(ctx context.Context, gateway Gateway, route Route) (bool, error)) ([]Gateway, []Route, error) {
	modifiedGateways := map[core.GatewayID]Gateway{}
	modifiedRoutes := map[string]Route{}
	for _, gateway := range gateways {
		for _, route := range routes {
			modified, err := f(ctx, gateway, route)
			if err != nil {
				return nil, nil, err
			}
			if modified {
				modifiedGateways[gateway.ID()] = gateway
				modifiedRoutes[route.ID()] = route
			}
		}
	}

	flattenedGateways := []Gateway{}
	flattenedRoutes := []Route{}
	for _, gateway := range modifiedGateways {
		flattenedGateways = append(flattenedGateways, gateway)
	}
	for _, route := range modifiedRoutes {
		flattenedRoutes = append(flattenedRoutes, route)
	}
	return flattenedGateways, flattenedRoutes, nil
}

func (s *store) bindAll(ctx context.Context, gateways []Gateway, routes []Route) ([]Gateway, []Route, error) {
	return bindOrUnbindAll(ctx, gateways, routes, s.binder.Bind)
}

func (s *store) unbindAll(ctx context.Context, gateways []Gateway, routes []Route) ([]Gateway, []Route, error) {
	return bindOrUnbindAll(ctx, gateways, routes, s.binder.Unbind)
}

func (s *store) upsertAll(ctx context.Context, gateways []Gateway, routes []Route) error {
	marshaledGateways := []GatewayRecord{}
	for _, gateway := range gateways {
		id := gateway.ID()
		data, err := s.marshaler.MarshalGateway(gateway)
		if err != nil {
			return err
		}
		marshaledGateways = append(marshaledGateways, GatewayRecord{
			ID:   id,
			Data: data,
		})
	}
	marshaledRoutes := []RouteRecord{}
	for _, route := range routes {
		id := route.ID()
		data, err := s.marshaler.MarshalRoute(route)
		if err != nil {
			return err
		}
		marshaledRoutes = append(marshaledRoutes, RouteRecord{
			ID:   id,
			Data: data,
		})
	}
	if err := s.backend.UpsertGateways(ctx, marshaledGateways...); err != nil {
		return err
	}
	return s.backend.UpsertRoutes(ctx, marshaledRoutes...)
}

func (s *store) syncGateway(ctx context.Context, gateway Gateway) error {
	if s.statusUpdater != nil {
		return s.statusUpdater.UpdateGatewayStatusOnSync(ctx, gateway, func() (bool, error) {
			return s.adapter.Sync(ctx, gateway.Resolve())
		})
	}
	_, err := s.adapter.Sync(ctx, gateway.Resolve())
	return err
}

func (s *store) syncGateways(ctx context.Context, gateways []Gateway) error {
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

func (s *store) syncRouteStatuses(ctx context.Context, routes []Route) error {
	if s.statusUpdater == nil {
		return nil
	}

	var syncGroup multierror.Group

	for _, r := range routes {
		route := r
		syncGroup.Go(func() error {
			return s.statusUpdater.UpdateRouteStatus(ctx, route)
		})
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
func (s *store) sync(ctx context.Context, gateways []Gateway, routes []Route) error {
	var syncGroup multierror.Group

	syncGroup.Go(func() error {
		return s.syncGateways(ctx, gateways)
	})
	syncGroup.Go(func() error {
		return s.syncRouteStatuses(ctx, routes)
	})
	if err := syncGroup.Wait().ErrorOrNil(); err != nil {
		return err
	}
	return nil
}
