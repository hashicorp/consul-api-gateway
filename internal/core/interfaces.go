package core

import (
	"context"
)

type CompareResult int

const (
	CompareResultInvalid CompareResult = iota
	CompareResultNewer
	CompareResultNotEqual
	CompareResultEqual
)

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
	ID() GatewayID
	Meta() map[string]string
	Compare(other Gateway) CompareResult
	Listeners() []Listener
	ShouldBind(route Route) bool
}

// ListenerConfig contains the common configuration
// options of a listener.
type ListenerConfig struct {
	Name     string
	Hostname string
	Port     int
	Protocol string
	TLS      bool
}

// Listener describes the basic methods of a gateway
// listener.
type Listener interface {
	ID() string
	CanBind(route Route) (bool, error)
	Certificates(ctx context.Context) ([]string, error)
	Config() ListenerConfig
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
	OnBound(gateway Gateway)
	OnBindFailed(err error, gateway Gateway)
	OnGatewayRemoved(gateway Gateway)
}

// Route should be implemented by all route
// source integrations
type Route interface {
	ID() string
	Compare(other Route) CompareResult
	Resolve(listener Listener) *ResolvedRoute
}

// SyncAdapter is used for synchronizing store state to
// an external system
type SyncAdapter interface {
	Sync(ctx context.Context, gateway ResolvedGateway) error
	Clear(ctx context.Context, id GatewayID) error
}

// Store is used for persisting and querying gateways and routes
type Store interface {
	GatewayExists(ctx context.Context, id GatewayID) (bool, error)
	CanFetchSecrets(ctx context.Context, id GatewayID, secrets []string) (bool, error)
	GetGateway(ctx context.Context, id GatewayID) (Gateway, error)
	DeleteGateway(ctx context.Context, id GatewayID) error
	UpsertGateway(ctx context.Context, gateway Gateway) error
	GetRoute(ctx context.Context, id string) (Route, error)
	DeleteRoute(ctx context.Context, id string) error
	UpsertRoute(ctx context.Context, route Route) error
	Sync(ctx context.Context) error
}
