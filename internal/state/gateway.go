package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-multierror"
)

type BoundGateway struct {
	Gateway

	consul    *api.Client
	listeners map[string]*BoundListener
	secrets   map[string]struct{}

	routers   *consul.ConfigEntryIndex
	splitters *consul.ConfigEntryIndex
	defaults  *consul.ConfigEntryIndex

	mutex sync.RWMutex
}

// NewBoundGateway creates a bound gateway
func NewBoundGateway(gateway Gateway, client *api.Client) *BoundGateway {
	listeners := make(map[string]*BoundListener)
	for _, listener := range gateway.Listeners() {
		listeners[listener.ID()] = NewBoundListener(gateway, listener)
	}

	secrets := make(map[string]struct{})
	for _, secret := range gateway.Secrets() {
		secrets[secret] = struct{}{}
	}

	return &BoundGateway{
		Gateway:   gateway,
		consul:    client,
		listeners: listeners,
		routers:   consul.NewConfigEntryIndex(api.ServiceRouter),
		splitters: consul.NewConfigEntryIndex(api.ServiceSplitter),
		defaults:  consul.NewConfigEntryIndex(api.ServiceDefaults),
		secrets:   secrets,
	}
}

func (g *BoundGateway) ResolveListenerTLS(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var result error
	for _, listener := range g.listeners {
		if err := listener.ResolveAndCacheTLS(ctx); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if result != nil {
		return result
	}
	return nil
}

// Merge merges consul tracking information from the supplied gateway.
func (g *BoundGateway) Merge(from *BoundGateway) {
	if from != nil {
		g.mutex.Lock()
		defer g.mutex.Unlock()

		from.mutex.RLock()
		defer from.mutex.RUnlock()

		g.defaults = from.defaults
		g.routers = from.routers
		g.splitters = from.splitters
	}
}

// Remove removes a route from the gateway's listeners if
// it is bound to a listener
func (g *BoundGateway) Remove(id string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, listener := range g.listeners {
		listener.RemoveRoute(id)
	}
}

func (g *BoundGateway) TryBind(route Route) {
	if g.ShouldBind(route) {
		bound := false
		var bindError error
		for _, l := range g.listeners {
			didBind, err := l.Bind(route)
			if err != nil {
				// consider each route distinct for the purposes of binding
				bindError = multierror.Append(bindError, err)
			}
			if didBind {
				l.SetRoute(route)
				bound = true
			}
		}
		if !bound {
			route.OnBindFailed(bindError, g.Gateway)
		} else {
			route.OnBound(g.Gateway)
		}
	}
}

func (g *BoundGateway) Sync(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.Logger().Trace("syncing gateway")
	for _, listener := range g.listeners {
		if listener.ShouldSync() {
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

func (g *BoundGateway) setConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, _, err := g.consul.ConfigEntries().Set(entry, options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (g *BoundGateway) deleteConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, err := g.consul.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (g *BoundGateway) sync(ctx context.Context) error {
	if g.Logger().IsTrace() {
		started := time.Now()
		g.Logger().Trace("started reconciliation", "time", started)
		defer g.Logger().Trace("reconciliation finished", "time", time.Now(), "spent", time.Since(started))
	}

	ingress := &api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
		Name: g.Name(),
		// TODO: namespaces
		Meta: g.Meta(),
	}

	computedRouters := consul.NewConfigEntryIndex(api.ServiceRouter)
	computedSplitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	computedDefaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	for _, l := range g.listeners {
		listener, routers, splitters, defaults := l.DiscoveryChain()
		if len(listener.Services) > 0 {
			// Consul requires that we have something to route to
			computedRouters.Merge(routers)
			computedSplitters.Merge(splitters)
			computedDefaults.Merge(defaults)
			ingress.Listeners = append(ingress.Listeners, listener)
		} else {
			g.Logger().Debug("listener has no services", "name", l.name)
		}
	}

	// Since we can't make multiple config entry changes in a single transaction we must
	// perform the operations in a set that is least likely to induce downtime.
	// First the new service-defaults, routers and splitters should be set
	// Second the ingress gateway
	// Third the removal of any service-defaults, routers or splitters that no longer exist

	addedRouters := computedRouters.ToArray()
	addedDefaults := computedDefaults.ToArray()
	addedSplitters := computedSplitters.ToArray()
	removedRouters := computedRouters.Difference(g.routers).ToArray()
	removedSplitters := computedSplitters.Difference(g.splitters).ToArray()
	removedDefaults := computedDefaults.Difference(g.defaults).ToArray()

	if g.Logger().IsTrace() {
		ingressEntry, err := json.MarshalIndent(ingress, "", "  ")
		if err == nil {
			g.Logger().Trace("setting ingress", "items", string(ingressEntry))
		}
		removed, err := json.MarshalIndent(append(append(removedRouters, removedSplitters...), removedDefaults...), "", "  ")
		if err == nil {
			g.Logger().Trace("removing", "items", string(removed))
		}
		added, err := json.MarshalIndent(append(append(addedRouters, addedSplitters...), addedDefaults...), "", "  ")
		if err == nil {
			g.Logger().Trace("adding", "items", string(added))
		}
	}

	// defaults need to go first, otherwise the routers are always configured to use tcp
	if err := g.setConfigEntries(ctx, addedDefaults...); err != nil {
		return fmt.Errorf("error adding service defaults config entries: %w", err)
	}
	if err := g.setConfigEntries(ctx, addedRouters...); err != nil {
		return fmt.Errorf("error adding service router config entries: %w", err)
	}
	if err := g.setConfigEntries(ctx, addedSplitters...); err != nil {
		return fmt.Errorf("error adding service splitter config entries: %w", err)
	}

	if err := g.setConfigEntries(ctx, ingress); err != nil {
		return fmt.Errorf("error adding ingress config entry: %w", err)
	}

	if err := g.deleteConfigEntries(ctx, removedRouters...); err != nil {
		return fmt.Errorf("error removing service router config entries: %w", err)
	}
	if err := g.deleteConfigEntries(ctx, removedSplitters...); err != nil {
		return fmt.Errorf("error removing service splitter config entries: %w", err)
	}
	if err := g.deleteConfigEntries(ctx, removedDefaults...); err != nil {
		return fmt.Errorf("error removing service defaults config entries: %w", err)
	}

	g.routers = computedRouters
	g.splitters = computedSplitters
	g.defaults = computedDefaults

	return nil
}
