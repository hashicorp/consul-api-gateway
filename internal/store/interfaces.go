package store

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

//go:generate mockgen -source ./interfaces.go -destination ./mocks/interfaces.go -package mocks StatusTrackingGateway,Gateway,RouteTrackingListener,Listener,StatusTrackingRoute,Route,Store

// Gateway describes a gateway.
type Gateway interface {
	ID() core.GatewayID
	Resolve() core.ResolvedGateway
	CanFetchSecrets(secrets []string) bool
}

// Route should be implemented by all route
// source integrations
type Route interface {
	ID() string
}

type GatewayRecord struct {
	ID   core.GatewayID
	Data []byte
}

type RouteRecord struct {
	ID   string
	Data []byte
}

// Backend is used for persisting and querying gateways and routes
type Backend interface {
	GetGateway(ctx context.Context, id core.GatewayID) ([]byte, error)
	GetGateways(ctx context.Context) ([][]byte, error)
	DeleteGateway(ctx context.Context, id core.GatewayID) error
	UpsertGateways(ctx context.Context, gateways ...GatewayRecord) error
	GetRoute(ctx context.Context, id string) ([]byte, error)
	GetRoutes(ctx context.Context) ([][]byte, error)
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

type Store interface {
	GetGateway(ctx context.Context, id core.GatewayID) (Gateway, error)
	GetGateways(ctx context.Context) ([]Gateway, error)
	UpsertGateway(ctx context.Context, gateway Gateway, updateConditionFn func(current Gateway) bool) error
	DeleteGateway(ctx context.Context, id core.GatewayID) error
	GetRoute(ctx context.Context, id string) (Route, error)
	GetRoutes(ctx context.Context) ([]Route, error)
	UpsertRoute(ctx context.Context, route Route, updateConditionFn func(current Route) bool) error
	DeleteRoute(ctx context.Context, id string) error
}
