// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/exp/maps"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

var (
	_ Store = (*lockingStore)(nil)

	ErrNotFound = errors.New("not found")

	consulSyncInterval = 60 * time.Second
	startSyncLoopOnce  sync.Once
)

// lockingStore is a wrapper around store that synchronizes reads + writes
type lockingStore struct {
	*store

	mutex sync.RWMutex
}

type Config struct {
	Adapter       core.SyncAdapter
	Backend       Backend
	Binder        Binder
	Logger        hclog.Logger
	Marshaler     Marshaler
	StatusUpdater StatusUpdater
}

func New(c Config) *lockingStore {
	return &lockingStore{
		store: &store{
			adapter:       c.Adapter,
			backend:       c.Backend,
			binder:        c.Binder,
			logger:        c.Logger,
			marshaler:     c.Marshaler,
			statusUpdater: c.StatusUpdater,
		},
	}
}

// GetGateway returns the Gateway with the requested ID if one exists
func (s *lockingStore) GetGateway(ctx context.Context, id core.GatewayID) (Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.getGateway(ctx, id)
}

// ListGateways returns a list of all Gateway(s) present in the persistent Backend
func (s *lockingStore) ListGateways(ctx context.Context) ([]Gateway, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.listGateways(ctx)
}

func (s *lockingStore) UpsertGateway(ctx context.Context, gateway Gateway, updateConditionFn func(current Gateway) bool) error {
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

	routes, err := s.listRoutes(ctx)
	if err != nil {
		return err
	}

	// Attempt to bind all Route(s) to the Gateway in case any are newly-able to bind
	_, boundRoutes := s.bindAll(ctx, []Gateway{gateway}, routes)

	if err := s.upsertGatewaysAndRoutes(ctx, []Gateway{gateway}, boundRoutes); err != nil {
		return err
	}

	return s.syncGatewaysAndRoutes(ctx, []Gateway{gateway}, boundRoutes)
}

// DeleteGateway unbinds any Route(s) that are bound to the Gateway
// and then deletes the Gateway from the persistent Backend.
func (s *lockingStore) DeleteGateway(ctx context.Context, id core.GatewayID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	gateway, err := s.getGateway(ctx, id)
	if err != nil || gateway == nil {
		return err
	}

	s.logger.Trace("Deleting gateway", "id", id)

	if err := s.adapter.Clear(ctx, id); err != nil {
		return err
	}

	routes, err := s.listRoutes(ctx)
	if err != nil {
		return err
	}

	// Unbind any Route(s) that are bound to the Gateway
	_, modifiedRoutes := s.unbindAll(ctx, []Gateway{gateway}, routes)

	if err = s.backend.DeleteGateway(ctx, id); err != nil {
		return err
	}

	if err = s.upsertRoutes(ctx, modifiedRoutes); err != nil {
		return err
	}

	return s.syncRouteStatuses(ctx, modifiedRoutes)
}

// GetRoute returns the Route with the requested ID if one exists
func (s *lockingStore) GetRoute(ctx context.Context, id string) (Route, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.getRoute(ctx, id)
}

// ListRoutes returns a list of all Route(s) present in the persistent Backend
func (s *lockingStore) ListRoutes(ctx context.Context) ([]Route, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	datas, err := s.backend.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	routes := make([]Route, len(datas))
	for i, data := range datas {
		route, err := s.marshaler.UnmarshalRoute(data)
		if err != nil {
			return nil, err
		}
		routes[i] = route
	}

	return routes, nil
}

func (s *lockingStore) UpsertRoute(ctx context.Context, route Route, updateConditionFn func(current Route) bool) error {
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

	gateways, err := s.listGateways(ctx)
	if err != nil {
		return err
	}

	modifiedGateways, _ := s.bindAll(ctx, gateways, []Route{route})

	if err = s.upsertGatewaysAndRoutes(ctx, modifiedGateways, []Route{route}); err != nil {
		return err
	}

	return s.syncGatewaysAndRoutes(ctx, modifiedGateways, []Route{route})
}

// DeleteRoute unbinds the Route from any Gateway(s) that it is bound to
// and then deletes the Route from the persistence Backend.
func (s *lockingStore) DeleteRoute(ctx context.Context, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	route, err := s.getRoute(ctx, id)
	if err != nil || route == nil {
		return err
	}

	gateways, err := s.listGateways(ctx)
	if err != nil {
		return err
	}

	// Unbind this Route from any Gateways it's bound to
	modifiedGateways, _ := s.unbindAll(ctx, gateways, []Route{route})

	if err = s.backend.DeleteRoute(ctx, id); err != nil {
		return err
	}

	if err = s.upsertGateways(ctx, modifiedGateways); err != nil {
		return err
	}

	return s.syncGateways(ctx, modifiedGateways)
}

// SyncAllAtInterval syncs all known Gateway(s) and Route(s) into Consul
// at a regular interval. This function is blocking.
func (s *lockingStore) SyncAllAtInterval(ctx context.Context) {
	startSyncLoopOnce.Do(func() {
		ticker := time.NewTicker(consulSyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mutex.Lock()

				if err := s.syncAll(ctx); err != nil {
					s.logger.Warn("An error occurred during background sync, some changes may be out of sync", "error", err)
				} else {
					s.logger.Trace("Synced store in background")
				}

				s.mutex.Unlock()
			}
		}
	})
}

// store is an orchestration layer over the persistent Backend that
// handles additional business logic required for CRUD operations.
//
// For example, deleting a Gateway requires that all Route(s) which have
// been bound to it be unbound before the Gateway itself is deleted.
type store struct {
	logger        hclog.Logger
	adapter       core.SyncAdapter
	backend       Backend
	marshaler     Marshaler
	binder        Binder
	statusUpdater StatusUpdater
}

func (s *store) getGateway(ctx context.Context, id core.GatewayID) (Gateway, error) {
	data, err := s.backend.GetGateway(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return s.marshaler.UnmarshalGateway(data)
}

func (s *store) listGateways(ctx context.Context) ([]Gateway, error) {
	datas, err := s.backend.ListGateways(ctx)
	if err != nil {
		return nil, err
	}

	gateways := make([]Gateway, len(datas))
	for i, data := range datas {
		gateway, err := s.marshaler.UnmarshalGateway(data)
		if err != nil {
			return nil, err
		}
		gateways[i] = gateway
	}

	return gateways, nil
}

func (s *store) getRoute(ctx context.Context, id string) (Route, error) {
	data, err := s.backend.GetRoute(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return s.marshaler.UnmarshalRoute(data)
}

func (s *store) listRoutes(ctx context.Context) ([]Route, error) {
	datas, err := s.backend.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	routes := make([]Route, len(datas))
	for i, data := range datas {
		route, err := s.marshaler.UnmarshalRoute(data)
		if err != nil {
			return nil, err
		}
		routes[i] = route
	}

	return routes, nil
}

func (s *store) upsertGatewaysAndRoutes(ctx context.Context, gateways []Gateway, routes []Route) error {
	if err := s.upsertGateways(ctx, gateways); err != nil {
		return err
	}
	return s.upsertRoutes(ctx, routes)
}

func (s *store) upsertGateways(ctx context.Context, gateways []Gateway) error {
	records := make([]GatewayRecord, len(gateways))
	for i, gateway := range gateways {
		data, err := s.marshaler.MarshalGateway(gateway)
		if err != nil {
			return err
		}

		records[i] = GatewayRecord{ID: gateway.ID(), Data: data}
	}
	return s.backend.UpsertGateways(ctx, records...)
}

func (s *store) upsertRoutes(ctx context.Context, routes []Route) error {
	records := make([]RouteRecord, len(routes))
	for i, route := range routes {
		data, err := s.marshaler.MarshalRoute(route)
		if err != nil {
			return err
		}

		records[i] = RouteRecord{ID: route.ID(), Data: data}
	}
	return s.backend.UpsertRoutes(ctx, records...)
}

// bindAll will bind all Route(s) to all Gateway(s)
func (s *store) bindAll(ctx context.Context, gateways []Gateway, routes []Route) ([]Gateway, []Route) {
	return bindOrUnbindAll(ctx, gateways, routes, s.binder.Bind)
}

// unbindAll will unbind all Route(s) from all Gateway(s)
func (s *store) unbindAll(ctx context.Context, gateways []Gateway, routes []Route) ([]Gateway, []Route) {
	return bindOrUnbindAll(ctx, gateways, routes, s.binder.Unbind)
}

type bindUnbindFunc func(context.Context, Gateway, Route) bool

// bindOrUnbindAll will call the bindUnbindFunc for all Route(s) on all Gateway(s)
// and return the list of modified Gateway(s) and list of modified Route(s).
func bindOrUnbindAll(ctx context.Context, gateways []Gateway, routes []Route, f bindUnbindFunc) ([]Gateway, []Route) {
	modifiedGateways := map[core.GatewayID]Gateway{}
	modifiedRoutes := map[string]Route{}

	for _, gateway := range gateways {
		for _, route := range routes {
			modified := f(ctx, gateway, route)
			if modified {
				modifiedGateways[gateway.ID()] = gateway
				modifiedRoutes[route.ID()] = route
			}
		}
	}

	return maps.Values(modifiedGateways), maps.Values(modifiedRoutes)
}

// syncAll updates the Gateway and Route statuses for all Gateway(s) and Route(s)
// in the store and sync all Gateway(s) using the adapter.
func (s *store) syncAll(ctx context.Context) error {
	var eg multierror.Group

	eg.Go(func() error {
		gateways, err := s.listGateways(ctx)
		if err != nil {
			return err
		}
		return s.syncGateways(ctx, gateways)
	})

	eg.Go(func() error {
		routes, err := s.listRoutes(ctx)
		if err != nil {
			return err
		}
		return s.syncRouteStatuses(ctx, routes)
	})

	return eg.Wait().ErrorOrNil()
}

// syncGatewaysAndRoutes updates the Gateway and Route statuses and syncs Gateways using the adapter.
//
// Care needs to be taken here as we spin up multiple goroutines to handle
// synchronization in parallel. Since we pass some objects from our internal
// state to callbacks, it means we *must not* access any potential references
// stored from previous callbacks.
func (s *store) syncGatewaysAndRoutes(ctx context.Context, gateways []Gateway, routes []Route) error {
	var eg multierror.Group

	eg.Go(func() error {
		return s.syncGateways(ctx, gateways)
	})

	eg.Go(func() error {
		return s.syncRouteStatuses(ctx, routes)
	})

	return eg.Wait().ErrorOrNil()
}

func (s *store) syncGateways(ctx context.Context, gateways []Gateway) error {
	var eg multierror.Group

	for _, gw := range gateways {
		gateway := gw
		eg.Go(func() error {
			return s.syncGateway(ctx, gateway)
		})
	}

	return eg.Wait().ErrorOrNil()
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
		s.logger.Error("Failed to syncGatewaysAndRoutes route statuses", "error", err)
		return err
	}

	return nil
}
