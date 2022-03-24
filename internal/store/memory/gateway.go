package memory

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
)

type gatewayState struct {
	store.Gateway

	logger    hclog.Logger
	adapter   core.SyncAdapter
	listeners map[string]*listenerState
	secrets   map[string]struct{}
}

// newGatewayState creates a bound gateway
func newGatewayState(logger hclog.Logger, gateway store.Gateway, adapter core.SyncAdapter) *gatewayState {
	id := gateway.ID()

	secrets := make(map[string]struct{})
	gatewayLogger := logger.With("gateway.consul.namespace", id.ConsulNamespace, "gateway.consul.service", id.Service)
	listeners := make(map[string]*listenerState)
	for _, listener := range gateway.Listeners() {
		for _, cert := range listener.Config().TLS.Certificates {
			secrets[cert] = struct{}{}
		}
		listeners[listener.ID()] = newListenerState(gatewayLogger, gateway, listener)
	}

	return &gatewayState{
		Gateway:   gateway,
		logger:    gatewayLogger,
		adapter:   adapter,
		listeners: listeners,
		secrets:   secrets,
	}
}

// Remove removes a route from the gateway's listeners if
// it is bound to a listener
func (g *gatewayState) Remove(id string) {
	for _, listener := range g.listeners {
		listener.RemoveRoute(id)
	}
}

func (g *gatewayState) TryBind(ctx context.Context, route store.Route) {
	g.logger.Trace("checking if route can bind to gateway", "route", route.ID())
	for _, name := range g.Bind(ctx, route) {
		if listener, ok := g.listeners[name]; ok {
			g.logger.Trace("setting listener route", "listener", listener.name, "route", route.ID())
			listener.SetRoute(route)
		}
	}
}

func (g *gatewayState) Sync(ctx context.Context) (bool, error) {
	didSync := false
	for _, listener := range g.listeners {
		if listener.ShouldSync() {
			g.logger.Trace("syncing gateway")
			if err := g.sync(ctx); err != nil {
				return false, err
			}
			didSync = true
			break
		}
	}

	for _, listener := range g.listeners {
		listener.MarkSynced()
	}

	return didSync, nil
}

func (g *gatewayState) sync(ctx context.Context) error {
	return g.adapter.Sync(ctx, g.Resolve())
}

func (g *gatewayState) Resolve() core.ResolvedGateway {
	listeners := []core.ResolvedListener{}
	for _, listener := range g.listeners {
		if listener.Listener.IsValid() {
			listeners = append(listeners, listener.Resolve())
		}
	}
	return core.ResolvedGateway{
		ID:        g.ID(),
		Meta:      g.Meta(),
		Listeners: listeners,
	}
}
