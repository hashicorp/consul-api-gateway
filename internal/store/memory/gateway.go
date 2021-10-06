package memory

import (
	"context"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

type gatewayState struct {
	core.Gateway

	logger    hclog.Logger
	adapter   core.SyncAdapter
	listeners map[string]*listenerState
	secrets   map[string]struct{}

	mutex sync.RWMutex
}

// newGatewayState creates a bound gateway
func newGatewayState(logger hclog.Logger, gateway core.Gateway, adapter core.SyncAdapter) *gatewayState {
	id := gateway.ID()

	gatewayLogger := logger.With("gateway.consul.namespace", id.ConsulNamespace, "gateway.consul.service", id.Service)
	listeners := make(map[string]*listenerState)
	for _, listener := range gateway.Listeners() {
		listeners[listener.ID()] = newListenerState(gatewayLogger, gateway, listener)
	}

	return &gatewayState{
		Gateway:   gateway,
		logger:    gatewayLogger,
		adapter:   adapter,
		listeners: listeners,
		secrets:   make(map[string]struct{}),
	}
}

func (g *gatewayState) ResolveListenerTLS(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var result error
	for _, listener := range g.listeners {
		certificates, err := listener.ResolveAndCacheTLS(ctx)
		if err != nil {
			result = multierror.Append(result, err)
			continue
		}
		for _, cert := range certificates {
			g.secrets[cert] = struct{}{}
		}
	}
	if result != nil {
		return result
	}
	return nil
}

// Remove removes a route from the gateway's listeners if
// it is bound to a listener
func (g *gatewayState) Remove(id string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, listener := range g.listeners {
		listener.RemoveRoute(id)
	}
}

func (g *gatewayState) TryBind(route core.Route) {
	if g.ShouldBind(route) {
		bound := false
		var bindError error
		for _, l := range g.listeners {
			canBind, err := l.CanBind(route)
			if err != nil {
				// consider each route distinct for the purposes of binding
				bindError = multierror.Append(bindError, err)
			}
			if canBind {
				l.SetRoute(route)
				bound = true
			}
		}
		if tracker, ok := route.(core.StatusTrackingRoute); ok {
			if !bound {
				tracker.OnBindFailed(bindError, g.Gateway)
			} else {
				tracker.OnBound(g.Gateway)
			}
		}
	}
}

func (g *gatewayState) Compare(other core.Gateway) core.CompareResult {
	if other == nil {
		return core.CompareResultInvalid
	}
	if g == nil {
		return core.CompareResultNotEqual
	}

	return g.Gateway.Compare(other)
}

func (g *gatewayState) Sync(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, listener := range g.listeners {
		if listener.ShouldSync() {
			g.logger.Trace("syncing gateway")
			if err := g.sync(ctx); err != nil {
				return err
			}
			break
		}
	}

	for _, listener := range g.listeners {
		listener.MarkSynced()
	}

	return nil
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
