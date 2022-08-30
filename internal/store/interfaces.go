package store

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

//go:generate mockgen -source ./interfaces.go -destination ./mocks/interfaces.go -package mocks StatusTrackingGateway,Gateway,RouteTrackingListener,Listener,StatusTrackingRoute,Route,Store

type CompareResult int

// StatusTrackingGateway is an optional extension
// of Gateway. If supported by a Store, when
// a Gateway is synced to an external location,
// its corresponding callbacks should
// be called.
type StatusTrackingGateway interface {
	Gateway

	TrackSync(ctx context.Context, sync func() (bool, error)) error
}

// Gateway describes a gateway.
type Gateway interface {
	ID() core.GatewayID
	Bind(ctx context.Context, route Route) []string
	Remove(ctx context.Context, id string) error
	Resolve() core.ResolvedGateway
	CanFetchSecrets(ctx context.Context, secrets []string) (bool, error)
}

// ListenerConfig contains the common configuration
// options of a listener.
type ListenerConfig struct {
	Name     string
	Hostname string
	Port     int
	Protocol string
	TLS      core.TLSParams
}

// RouteTrackingListener is an optional extension
// to Listener that tracks when routes have been
// bound to it.
type RouteTrackingListener interface {
	Listener

	OnRouteAdded(route Route)
	OnRouteRemoved(id string)
}

// Listener describes the basic methods of a gateway
// listener.
type Listener interface {
	ID() string
	CanBind(ctx context.Context, route Route) (bool, error)
	Config() ListenerConfig
	IsValid() bool
}

// StatusTrackingRoute is an optional extension
// of Route. If supported by a Store, when
// a Route is bound or fails to be bound to
// a gateway, its corresponding callbacks should
// be called. At the end of any methods affecting
// the route's binding, SyncStatus should be called.
type StatusTrackingRoute interface {
	Route

	SyncStatus(ctx context.Context) error
	OnGatewayRemoved(gateway Gateway)
}

// Route should be implemented by all route
// source integrations
type Route interface {
	ID() string
}

// Store is used for persisting and querying gateways and routes
type Store interface {
	GetGateway(ctx context.Context, id core.GatewayID) (Gateway, error)
	DeleteGateway(ctx context.Context, id core.GatewayID) error
	UpsertGateway(ctx context.Context, gateway Gateway, updateConditionFn func(current Gateway) bool) error
	DeleteRoute(ctx context.Context, id string) error
	UpsertRoute(ctx context.Context, route Route, updateConditionFn func(current Route) bool) error
	Sync(ctx context.Context) error
}
