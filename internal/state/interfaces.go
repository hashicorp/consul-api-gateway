package state

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

// GatewayID encapsulates enough information
// to describe a particular deployed gateway
type GatewayID struct {
	ConsulNamespace string
	Service         string
}

type Gateway interface {
	Logger() hclog.Logger

	ID() GatewayID
	Name() string
	Namespace() string
	Meta() map[string]string
	IsMoreRecent(other Gateway) bool
	Listeners() []Listener
	Equals(other Gateway) bool
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

type Route interface {
	Logger() hclog.Logger

	ID() string
	IsMoreRecent(other Route) bool
	Equals(other Route) bool
	ResolveServices(ctx context.Context) error
	DiscoveryChain(listener Listener) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex)
	SyncStatus(ctx context.Context) error
	OnBound(gateway Gateway)
	OnBindFailed(err error, gateway Gateway)
	OnGatewayRemoved(gateway Gateway)
}
