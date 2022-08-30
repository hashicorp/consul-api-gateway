package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/core"
)

type syncState struct {
	routers   *consul.ConfigEntryIndex
	splitters *consul.ConfigEntryIndex
	defaults  *consul.ConfigEntryIndex
}

type SyncAdapter struct {
	logger hclog.Logger
	consul *api.Client

	sync       map[core.GatewayID]syncState
	intentions map[core.GatewayID]*consul.IntentionsReconciler
	mutex      sync.Mutex
}

var _ core.SyncAdapter = &SyncAdapter{}

func NewSyncAdapter(logger hclog.Logger, consulClient *api.Client) *SyncAdapter {
	return &SyncAdapter{
		logger:     logger,
		consul:     consulClient,
		sync:       make(map[core.GatewayID]syncState),
		intentions: make(map[core.GatewayID]*consul.IntentionsReconciler),
	}
}

func (a *SyncAdapter) setConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, _, err := a.consul.ConfigEntries().Set(entry, options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (a *SyncAdapter) deleteConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		options.Namespace = entry.GetNamespace()
		if _, err := a.consul.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func routeDiscoveryChain(route core.ResolvedRoute) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	meta := route.GetMeta()
	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	switch route.GetType() {
	case core.ResolvedHTTPRouteType:
		httpRoute := route.(core.HTTPRoute)
		router, splits := httpRouteDiscoveryChain(httpRoute)
		serviceDefault := httpServiceDefault(router, meta)
		defaults.Add(serviceDefault)
		for _, split := range splits {
			splitters.Add(split)
			if split.Name != serviceDefault.Name {
				defaults.Add(httpServiceDefault(split, meta))
			}
		}

		return &api.IngressService{
			Name:      router.Name,
			Hosts:     httpRoute.Hostnames,
			Namespace: httpRoute.GetNamespace(),
		}, router, splitters, defaults
	case core.ResolvedTCPRouteType:
		tcpRoute := route.(core.TCPRoute)
		return &api.IngressService{
			Name:      tcpRoute.Service.Service,
			Namespace: tcpRoute.Service.ConsulNamespace,
		}, nil, nil, nil
	default:
		return nil, nil, nil, nil
	}
}

func flattenHTTPRoutes(gateway core.ResolvedGateway, resolved []core.ResolvedRoute) []core.ResolvedRoute {
	consolidator := newRouteConsolidator()
	unmerged := []core.ResolvedRoute{}
	for _, route := range resolved {
		switch route.GetType() {
		case core.ResolvedHTTPRouteType:
			consolidator.add(route.(core.HTTPRoute))
		default:
			unmerged = append(unmerged, route)
		}
	}

	for _, route := range consolidator.consolidate(gateway) {
		unmerged = append(unmerged, route)
	}
	return unmerged
}

// filterTCPRoutes makes sure we only have a single TCPRoute for a given listener
func filterTCPRoutes(routes []core.ResolvedRoute) []core.ResolvedRoute {
	filtered := []core.ResolvedRoute{}

	found := false
	for _, route := range routes {
		switch route.GetType() {
		case core.ResolvedTCPRouteType:
			if !found {
				found = true
				filtered = append(filtered, route)
			}
		default:
			filtered = append(filtered, route)
		}
	}

	return filtered
}

func mergeRoutes(gateway core.ResolvedGateway, routes []core.ResolvedRoute) []core.ResolvedRoute {
	routes = flattenHTTPRoutes(gateway, routes)
	return filterTCPRoutes(routes)
}

func discoveryChain(gateway core.ResolvedGateway) (*api.IngressGatewayConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	ingress := &api.IngressGatewayConfigEntry{
		Kind:      api.IngressGateway,
		Name:      gateway.ID.Service,
		Namespace: gateway.ID.ConsulNamespace,
		Meta:      gateway.Meta,
	}
	routers := consul.NewConfigEntryIndex(api.ServiceRouter)
	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	for _, listener := range gateway.Listeners {
		services := []api.IngressService{}

		routes := mergeRoutes(gateway, listener.Routes)
		for _, route := range routes {
			service, router, splits, serviceDefaults := routeDiscoveryChain(route)
			if service != nil {
				services = append(services, *service)
				if router != nil {
					routers.Add(router)
				}
				splitters.Merge(splits)
				defaults.Merge(serviceDefaults)
			}
		}

		if len(services) > 0 {
			tls := &api.GatewayTLSConfig{}

			if listener.TLS.MinVersion != "" {
				tls.TLSMinVersion = listener.TLS.MinVersion
			} else {
				// set secure default instead of Envoy's TLS 1.0 default
				tls.TLSMinVersion = "TLSv1_2"
			}

			if listener.TLS.MaxVersion != "" {
				tls.TLSMaxVersion = listener.TLS.MaxVersion
			}

			if len(listener.TLS.CipherSuites) > 0 {
				tls.CipherSuites = listener.TLS.CipherSuites
			} else {
				// set secure defaults excluding insecure RSA and SHA-1 ciphers pending removal from Envoy
				tls.CipherSuites = common.DefaultTLSCipherSuites()
			}

			if len(listener.TLS.Certificates) > 0 {
				tls.SDS = &api.GatewayTLSSDSConfig{
					ClusterName:  "sds-cluster",
					CertResource: listener.TLS.Certificates[0],
				}
			}

			ingress.Listeners = append(ingress.Listeners, api.IngressListener{
				Port:     listener.Port,
				Protocol: listener.Protocol,
				Services: services,
				TLS:      tls,
			})
		}
	}

	return ingress, routers, splitters, defaults
}

func (a *SyncAdapter) entriesForGateway(id core.GatewayID) (*consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	existing, found := a.sync[id]
	if !found {
		routers := consul.NewConfigEntryIndex(api.ServiceRouter)
		splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
		defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)
		return routers, splitters, defaults
	}
	return existing.routers, existing.splitters, existing.defaults
}

func (a *SyncAdapter) setEntriesForGateway(gateway core.ResolvedGateway, routers *consul.ConfigEntryIndex, splitters *consul.ConfigEntryIndex, defaults *consul.ConfigEntryIndex) {
	a.sync[gateway.ID] = syncState{
		routers:   routers,
		splitters: splitters,
		defaults:  defaults,
	}
}

func (a *SyncAdapter) syncIntentionsForGateway(gateway core.GatewayID, ingress *api.IngressGatewayConfigEntry) error {
	if a.intentions[gateway] == nil {
		a.intentions[gateway] = consul.NewIntentionsReconciler(a.consul, ingress, a.logger)
	} else {
		a.intentions[gateway].SetIngressServices(ingress)
	}
	return a.intentions[gateway].Reconcile()
}

func (a *SyncAdapter) stopIntentionSyncForGateway(gw core.GatewayID) {
	if ir, ok := a.intentions[gw]; ok {
		ir.Stop()
		delete(a.intentions, gw)
	}
}

func (a *SyncAdapter) Clear(ctx context.Context, id core.GatewayID) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if _, found := a.sync[id]; !found {
		return nil
	}

	if a.logger.IsTrace() {
		started := time.Now()
		a.logger.Trace("clearing entries for gateway", "time", started)
		defer a.logger.Trace("entries cleared", "time", time.Now(), "spent", time.Since(started))
	}

	ingress := &api.IngressGatewayConfigEntry{
		Kind:      api.IngressGateway,
		Name:      id.Service,
		Namespace: id.ConsulNamespace,
	}
	existingRouters, existingSplitters, existingDefaults := a.entriesForGateway(id)
	removedRouters := existingRouters.ToArray()
	removedSplitters := existingSplitters.ToArray()
	removedDefaults := existingDefaults.ToArray()

	if a.logger.IsTrace() {
		ingressEntry, err := json.MarshalIndent(ingress, "", "  ")
		if err == nil {
			a.logger.Trace("removing ingress", "items", string(ingressEntry))
		}
		removed, err := json.MarshalIndent(append(append(removedRouters, removedSplitters...), removedDefaults...), "", "  ")
		if err == nil {
			a.logger.Trace("removing", "items", string(removed))
		}
	}

	// remove the ingress config entry first so that it doesn't throw errors
	// when the defaults are removed due to protocol mismatches
	if err := a.deleteConfigEntries(ctx, ingress); err != nil {
		return fmt.Errorf("error removing ingress config entry: %w", err)
	}

	if err := a.deleteConfigEntries(ctx, removedRouters...); err != nil {
		return fmt.Errorf("error removing service router config entries: %w", err)
	}
	if err := a.deleteConfigEntries(ctx, removedSplitters...); err != nil {
		return fmt.Errorf("error removing service splitter config entries: %w", err)
	}
	if err := a.deleteConfigEntries(ctx, removedDefaults...); err != nil {
		return fmt.Errorf("error removing service defaults config entries: %w", err)
	}

	a.stopIntentionSyncForGateway(id)
	delete(a.sync, id)
	return nil
}

func (a *SyncAdapter) Sync(ctx context.Context, gateway core.ResolvedGateway) (bool, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.logger.IsTrace() {
		started := time.Now()
		resolved, err := json.MarshalIndent(gateway, "", "  ")
		if err == nil {
			a.logger.Trace("reconciling gateway snapshot", "gateway", string(resolved))
		}
		a.logger.Trace("started reconciliation", "time", started)
		defer a.logger.Trace("reconciliation finished", "time", time.Now(), "spent", time.Since(started))
	}

	ingress, computedRouters, computedSplitters, computedDefaults := discoveryChain(gateway)
	existingRouters, existingSplitters, existingDefaults := a.entriesForGateway(gateway.ID)

	// Since we can't make multiple config entry changes in a single transaction we must
	// perform the operations in a set that is least likely to induce downtime.
	// First the new service-defaults, routers and splitters should be set
	// Second the ingress gateway
	// Third the removal of any service-defaults, routers or splitters that no longer exist

	addedRouters := computedRouters.ToArray()
	addedDefaults := computedDefaults.ToArray()
	addedSplitters := computedSplitters.ToArray()
	removedRouters := computedRouters.Difference(existingRouters).ToArray()
	removedSplitters := computedSplitters.Difference(existingSplitters).ToArray()
	removedDefaults := computedDefaults.Difference(existingDefaults).ToArray()

	if a.logger.IsTrace() {
		ingressEntry, err := json.MarshalIndent(ingress, "", "  ")
		if err == nil {
			a.logger.Trace("setting ingress", "items", string(ingressEntry))
		}
		removed, err := json.MarshalIndent(append(append(removedRouters, removedSplitters...), removedDefaults...), "", "  ")
		if err == nil {
			a.logger.Trace("removing", "items", string(removed))
		}
		added, err := json.MarshalIndent(append(append(addedRouters, addedSplitters...), addedDefaults...), "", "  ")
		if err == nil {
			a.logger.Trace("adding", "items", string(added))
		}
	}

	// defaults need to go first, otherwise the routers are always configured to use tcp
	if err := a.setConfigEntries(ctx, addedDefaults...); err != nil {
		return false, fmt.Errorf("error adding service defaults config entries: %w", err)
	}
	if err := a.setConfigEntries(ctx, addedRouters...); err != nil {
		return false, fmt.Errorf("error adding service router config entries: %w", err)
	}
	if err := a.setConfigEntries(ctx, addedSplitters...); err != nil {
		return false, fmt.Errorf("error adding service splitter config entries: %w", err)
	}

	if err := a.setConfigEntries(ctx, ingress); err != nil {
		return false, fmt.Errorf("error adding ingress config entry: %w", err)
	}

	if err := a.deleteConfigEntries(ctx, removedRouters...); err != nil {
		return false, fmt.Errorf("error removing service router config entries: %w", err)
	}
	if err := a.deleteConfigEntries(ctx, removedSplitters...); err != nil {
		return false, fmt.Errorf("error removing service splitter config entries: %w", err)
	}
	if err := a.deleteConfigEntries(ctx, removedDefaults...); err != nil {
		return false, fmt.Errorf("error removing service defaults config entries: %w", err)
	}

	a.setEntriesForGateway(gateway, computedRouters, computedSplitters, computedDefaults)
	if err := a.syncIntentionsForGateway(gateway.ID, ingress); err != nil {
		return false, fmt.Errorf("error syncing service intention config entries: %w", err)
	}

	return true, nil
}
