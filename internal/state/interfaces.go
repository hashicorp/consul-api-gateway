package state

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type CompareResult int

const (
	CompareResultInvalid CompareResult = iota
	CompareResultNewer
	CompareResultNotEqual
	CompareResultEqual
)

type GatewayID struct {
	ConsulNamespace string
	Service         string
}

type Gateway interface {
	Logger() hclog.Logger

	ID() GatewayID
	Meta() map[string]string
	Compare(other Gateway) CompareResult
	Listeners() []Listener
	ShouldBind(route Route) bool
	Secrets() []string
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
	Bind(route Route) (bool, error)
	ResolveTLS(ctx context.Context) (*api.GatewayTLSConfig, error)
	Config() ListenerConfig
}

type StatusTrackingRoute interface {
	Route

	SyncStatus(ctx context.Context) error
	OnBound(gateway Gateway)
	OnBindFailed(err error, gateway Gateway)
	OnGatewayRemoved(gateway Gateway)
}

type Route interface {
	Logger() hclog.Logger

	ID() string
	Compare(other Route) CompareResult
	ResolveServices(ctx context.Context) error
	DiscoveryChain(listener Listener) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex)
}
