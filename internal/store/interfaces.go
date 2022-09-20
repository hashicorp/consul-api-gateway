package store

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

//go:generate mockgen -source ./interfaces.go -destination ./mocks/interfaces.go -package mocks StatusTrackingGateway,Gateway,StatusTrackingRoute,Route,Store

type CompareResult int

// StatusTrackingGateway is an optional extension
// of Gateway. If supported by a Store, when
// a Gateway is synced to an external location,
// its corresponding callbacks should
// be called.
// TODO Remove
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
	CanFetchSecrets(secrets []string) (bool, error)
}

type NewGateway interface {
	ID() core.GatewayID
	Resolve() core.ResolvedGateway
	CanFetchSecrets(secrets []string) (bool, error)
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
// TODO Remove
type Store interface {
	GetGateway(ctx context.Context, id core.GatewayID) (Gateway, error)
	DeleteGateway(ctx context.Context, id core.GatewayID) error
	UpsertGateway(ctx context.Context, gateway Gateway, updateConditionFn func(current Gateway) bool) error
	DeleteRoute(ctx context.Context, id string) error
	UpsertRoute(ctx context.Context, route Route, updateConditionFn func(current Route) bool) error
	Sync(ctx context.Context) error
}

// NewStore is used for persisting and querying gateways and routes
type NewStore interface {
	GetGateway(ctx context.Context, id core.GatewayID) (Gateway, error)
	ListGateways(ctx context.Context) ([]Gateway, error)
	UpsertGateway(ctx context.Context, gateway Gateway, updateConditionFn func(current Gateway) bool) error
	DeleteGateway(ctx context.Context, id core.GatewayID) error

	GetRoute(ctx context.Context, id string) (Route, error)
	ListRoutes(ctx context.Context) ([]Route, error)
	UpsertRoute(ctx context.Context, route Route, updateConditionFn func(current Route) bool) error
	DeleteRoute(ctx context.Context, id string) error

	SyncAllAtInterval(ctx context.Context)
}

// Backend is used for persisting and querying gateways and routes
type Backend interface {
	GetGateway(ctx context.Context, id core.GatewayID) ([]byte, error)
	ListGateways(ctx context.Context) ([][]byte, error)
	DeleteGateway(ctx context.Context, id core.GatewayID) error
	UpsertGateways(ctx context.Context, gateways ...GatewayRecord) error
	GetRoute(ctx context.Context, id string) ([]byte, error)
	ListRoutes(ctx context.Context) ([][]byte, error)
	DeleteRoute(ctx context.Context, id string) error
	UpsertRoutes(ctx context.Context, routes ...RouteRecord) error
}

type Marshaler interface {
	UnmarshalRoute(data []byte) (Route, error)
	MarshalRoute(Route) ([]byte, error)
	UnmarshalGateway(data []byte) (Gateway, error)
	MarshalGateway(Gateway) ([]byte, error)
}

type Binder interface {
	Bind(ctx context.Context, gateway Gateway, route Route) (bool, error)
	Unbind(ctx context.Context, gateway Gateway, route Route) (bool, error)
}

type StatusUpdater interface {
	UpdateGatewayStatusOnSync(ctx context.Context, gateway Gateway, sync func() (bool, error)) error
	UpdateRouteStatus(ctx context.Context, route Route) error
}

// GatewayRecord represents a serialized Gateway
type GatewayRecord struct {
	ID   core.GatewayID
	Data []byte
}

// RouteRecord represents a serialized Route
type RouteRecord struct {
	ID   string
	Data []byte
}
