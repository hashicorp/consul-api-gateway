package state

import (
	"context"

	"github.com/hashicorp/go-hclog"
)

type CompareResult int

const (
	CompareResultInvalid CompareResult = iota
	CompareResultNewer
	CompareResultNotEqual
	CompareResultEqual
)

type Gateway interface {
	Logger() hclog.Logger

	ID() GatewayID
	ConsulMeta() map[string]string
	Compare(other Gateway) CompareResult
	Listeners() []Listener
	ShouldBind(route Route) bool
}

type ListenerConfig struct {
	Name     string
	Hostname string
	Port     int
	Protocol string
	TLS      bool
}

type Listener interface {
	Logger() hclog.Logger

	ID() string
	CanBind(route Route) (bool, error)
	Certificates(ctx context.Context) ([]string, error)
	Config() ListenerConfig
}

type StatusTrackingRoute interface {
	Route

	SyncStatus(ctx context.Context) error
	OnBound(gateway Gateway)
	OnBindFailed(err error, gateway Gateway)
	OnGatewayRemoved(gateway Gateway)
}

type InitializableRoute interface {
	Route

	Init(ctx context.Context) error
}

type Route interface {
	Logger() hclog.Logger

	ID() string
	Compare(other Route) CompareResult
	Resolve(listener Listener) *ResolvedRoute
}

type SyncAdapter interface {
	Sync(ctx context.Context, gateway ResolvedGateway) error
}
