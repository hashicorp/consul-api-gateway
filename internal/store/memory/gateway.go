package memory

import (
	"context"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

type gatewayState struct {
	store.Gateway

	logger    hclog.Logger
	adapter   core.SyncAdapter
	listeners map[string]*listenerState
	secrets   map[string]struct{}
	needsSync bool
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
		needsSync: false,
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
	if g.ShouldBind(route) {
		bound := false
		var bindError error
		for _, l := range g.listeners {
			g.logger.Trace("checking if route can bind to listener", "listener", l.name, "route", route.ID())
			canBind, err := l.CanBind(ctx, route)
			if err != nil {
				// consider each route distinct for the purposes of binding
				g.logger.Debug("error binding route to gateway", "error", err, "route", route.ID())
				l.RemoveRoute(route.ID())
				bindError = multierror.Append(bindError, err)
			}
			if canBind {
				g.logger.Trace("setting listener route", "listener", l.name, "route", route.ID())
				l.SetRoute(route)
				bound = true
			}
		}
		if tracker, ok := route.(store.StatusTrackingRoute); ok {
			if !bound {
				tracker.OnBindFailed(bindError, g.Gateway)
			} else {
				tracker.OnBound(g.Gateway)
			}
		}
	} else {
		// Clean up route from gateway listeners if ParentRef no longer
		// references gateway
		g.Remove(route.ID())
	}
}

func (g *gatewayState) ShouldUpdate(other store.Gateway) bool {
	if other == nil {
		return false
	}
	if g == nil {
		return true
	}

	return g.Gateway.ShouldUpdate(other)
}

func (g *gatewayState) ShouldSync(ctx context.Context) bool {
	if g.needsSync {
		return true
	}

	for _, listener := range g.listeners {
		if listener.ShouldSync() {
			return true
		}
	}

	return false
}

func (g *gatewayState) MarkSynced() {
	g.needsSync = false
}

func (g *gatewayState) Sync(ctx context.Context) (bool, error) {
	didSync := false

	if g.ShouldSync(ctx) {
		g.logger.Trace("syncing gateway")
		if err := g.sync(ctx); err != nil {
			return false, err
		}
		didSync = true
	}

	g.MarkSynced()
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
