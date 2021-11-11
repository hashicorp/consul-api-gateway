package memory

import (
	"context"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
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
		for _, cert := range listener.Certificates() {
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

func (g *gatewayState) TryBind(route store.Route) {
	g.logger.Trace("checking if route can bind to gateway", "route", route.ID())
	if g.ShouldBind(route) {
		bound := false
		var bindError error
		for _, l := range g.listeners {
			g.logger.Trace("checking if route can bind to listener", "listener", l.name, "route", route.ID())
			canBind, err := l.CanBind(route)
			if err != nil {
				// consider each route distinct for the purposes of binding
				g.logger.Debug("error binding route to gateway", "error", err, "route", route.ID())
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
	}
}

func (g *gatewayState) Compare(other store.Gateway) store.CompareResult {
	if other == nil {
		return store.CompareResultInvalid
	}
	if g == nil {
		return store.CompareResultNotEqual
	}

	return g.Gateway.Compare(other)
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
		listeners = append(listeners, listener.Resolve())
	}
	return core.ResolvedGateway{
		ID:        g.ID(),
		Meta:      g.Meta(),
		Listeners: listeners,
	}
}
