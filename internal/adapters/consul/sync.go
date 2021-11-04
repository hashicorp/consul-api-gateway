package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

type syncState struct {
	routers   *consul.ConfigEntryIndex
	splitters *consul.ConfigEntryIndex
	defaults  *consul.ConfigEntryIndex
}

type ConsulSyncAdapter struct {
	logger hclog.Logger
	consul *api.Client

	sync       map[core.GatewayID]syncState
	intentions map[core.GatewayID]*consul.IntentionsReconciler
	mutex      sync.Mutex
}

var _ core.SyncAdapter = &ConsulSyncAdapter{}

func NewConsulSyncAdapter(logger hclog.Logger, consulClient *api.Client) *ConsulSyncAdapter {
	return &ConsulSyncAdapter{
		logger:     logger,
		consul:     consulClient,
		sync:       make(map[core.GatewayID]syncState),
		intentions: make(map[core.GatewayID]*consul.IntentionsReconciler),
	}
}

func (a *ConsulSyncAdapter) setConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, _, err := a.consul.ConfigEntries().Set(entry, options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (a *ConsulSyncAdapter) deleteConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, err := a.consul.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

// httpRouteToServiceDiscoChain will convert a k8s HTTPRoute to a Consul service-router config entry and 0 or
// more service-splitter config entries. A prefix can be given to prefix all config entry names with.
func httpRouteDiscoveryChain(route core.HTTPRoute) (*api.ServiceRouterConfigEntry, []*api.ServiceSplitterConfigEntry) {
	router := &api.ServiceRouterConfigEntry{
		Kind:      api.ServiceRouter,
		Name:      route.GetName(),
		Meta:      route.GetMeta(),
		Namespace: route.GetNamespace(),
	}
	var splitters []*api.ServiceSplitterConfigEntry

	for idx, rule := range route.Rules {
		modifier := httpRouteFiltersToServiceRouteHeaderModifier(rule.Filters)

		var destination core.ResolvedService
		if len(rule.Services) == 1 {
			destination = rule.Services[0].Service
			serviceModifier := httpRouteFiltersToServiceRouteHeaderModifier(rule.Services[0].Filters)
			modifier.Add = mergeMaps(modifier.Add, serviceModifier.Add)
			modifier.Set = mergeMaps(modifier.Set, serviceModifier.Set)
			modifier.Remove = append(modifier.Remove, serviceModifier.Remove...)
		} else {
			// create a virtual service to split
			destination = core.ResolvedService{
				Service:         fmt.Sprintf("%s-%d", route.GetName(), idx),
				ConsulNamespace: route.GetNamespace(),
			}
			splitter := &api.ServiceSplitterConfigEntry{
				Kind:      api.ServiceSplitter,
				Name:      destination.Service,
				Namespace: destination.ConsulNamespace,
				Splits:    []api.ServiceSplit{},
				Meta:      route.GetMeta(),
			}

			totalWeight := int32(0)
			for _, service := range rule.Services {
				totalWeight += service.Weight
			}

			for _, service := range rule.Services {
				if service.Weight == 0 {
					continue
				}

				modifier := httpRouteFiltersToServiceRouteHeaderModifier(service.Filters)

				weightPercentage := float32(service.Weight) / float32(totalWeight)
				split := api.ServiceSplit{
					RequestHeaders: modifier,
					Weight:         weightPercentage * 100,
				}
				split.Service = service.Service.Service
				split.Namespace = service.Service.ConsulNamespace
				splitter.Splits = append(splitter.Splits, split)
			}
			if len(splitter.Splits) > 0 {
				splitters = append(splitters, splitter)
			}
		}

		// for each match rule a ServiceRoute is created for the service-router
		// if there are no rules a single route with the destination is set
		if len(rule.Matches) == 0 {
			router.Routes = append(router.Routes, api.ServiceRoute{
				Destination: &api.ServiceRouteDestination{
					Service:        destination.Service,
					RequestHeaders: modifier,
					Namespace:      destination.ConsulNamespace,
				},
			})
		}
		for _, match := range rule.Matches {
			router.Routes = append(router.Routes, api.ServiceRoute{
				Match: &api.ServiceRouteMatch{HTTP: httpRouteMatchToServiceRouteHTTPMatch(match)},
				Destination: &api.ServiceRouteDestination{
					Service:        destination.Service,
					RequestHeaders: modifier,
					Namespace:      destination.ConsulNamespace,
				},
			})
		}
	}

	return router, splitters
}

func httpRouteFiltersToServiceRouteHeaderModifier(filters []core.HTTPFilter) *api.HTTPHeaderModifiers {
	modifier := &api.HTTPHeaderModifiers{
		Add: make(map[string]string),
		Set: make(map[string]string),
	}
	for _, filter := range filters {
		switch filter.Type {
		case core.HTTPHeaderFilterType:
			// If we have multiple filters specified, then we can potentially clobber
			// "Add" and "Set" here -- as far as K8S gateway spec is concerned, this
			// is all implmentation-specific behavior and undefined by the spec.
			modifier.Add = mergeMaps(modifier.Add, filter.Header.Add)
			modifier.Set = mergeMaps(modifier.Set, filter.Header.Set)
			modifier.Remove = append(modifier.Remove, filter.Header.Remove...)
		}
	}
	return modifier
}

func mergeMaps(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func httpRouteMatchToServiceRouteHTTPMatch(match core.HTTPMatch) *api.ServiceRouteHTTPMatch {
	var consulMatch api.ServiceRouteHTTPMatch
	switch match.Path.Type {
	case core.HTTPPathMatchExactType:
		consulMatch.PathExact = match.Path.Value
	case core.HTTPPathMatchPrefixType:
		consulMatch.PathPrefix = match.Path.Value
	case core.HTTPPathMatchRegularExpressionType:
		consulMatch.PathRegex = match.Path.Value
	}

	for _, header := range match.Headers {
		switch header.Type {
		case core.HTTPHeaderMatchExactType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:  header.Name,
				Exact: header.Value,
			})
		case core.HTTPHeaderMatchPrefixType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:   header.Name,
				Prefix: header.Value,
			})
		case core.HTTPHeaderMatchSuffixType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:   header.Name,
				Suffix: header.Value,
			})
		case core.HTTPHeaderMatchPresentType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:    header.Name,
				Present: true,
			})
		case core.HTTPHeaderMatchRegularExpressionType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:  header.Name,
				Regex: header.Value,
			})
		}
	}

	for _, query := range match.Query {
		switch query.Type {
		case core.HTTPQueryMatchExactType:
			consulMatch.QueryParam = append(consulMatch.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:  query.Name,
				Exact: query.Value,
			})
		case core.HTTPQueryMatchPresentType:
			consulMatch.QueryParam = append(consulMatch.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:    query.Name,
				Present: true,
			})
		case core.HTTPQueryMatchRegularExpressionType:
			consulMatch.QueryParam = append(consulMatch.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:  query.Name,
				Regex: query.Value,
			})
		}
	}

	switch match.Method {
	case core.HTTPMethodConnect:
		consulMatch.Methods = append(consulMatch.Methods, "CONNECT")
	case core.HTTPMethodDelete:
		consulMatch.Methods = append(consulMatch.Methods, "DELETE")
	case core.HTTPMethodGet:
		consulMatch.Methods = append(consulMatch.Methods, "GET")
	case core.HTTPMethodHead:
		consulMatch.Methods = append(consulMatch.Methods, "HEAD")
	case core.HTTPMethodOptions:
		consulMatch.Methods = append(consulMatch.Methods, "OPTIONS")
	case core.HTTPMethodPatch:
		consulMatch.Methods = append(consulMatch.Methods, "PATCH")
	case core.HTTPMethodPost:
		consulMatch.Methods = append(consulMatch.Methods, "POST")
	case core.HTTPMethodPut:
		consulMatch.Methods = append(consulMatch.Methods, "PUT")
	case core.HTTPMethodTrace:
		consulMatch.Methods = append(consulMatch.Methods, "TRACE")
	}

	return &consulMatch
}

func httpServiceDefault(entry api.ConfigEntry, meta map[string]string) *api.ServiceConfigEntry {
	return &api.ServiceConfigEntry{
		Kind:      api.ServiceDefaults,
		Name:      entry.GetName(),
		Namespace: entry.GetNamespace(),
		Protocol:  "http",
		Meta:      meta,
	}
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
	default:
		return nil, nil, nil, nil
	}
}

func mergeRoutes(gateway core.ResolvedGateway, routes []core.ResolvedRoute) []core.ResolvedRoute {
	merged := map[string]core.HTTPRoute{}
	unmerged := []core.ResolvedRoute{}
	for _, route := range routes {
		switch route.GetType() {
		case core.ResolvedHTTPRouteType:
			httpRoute := route.(core.HTTPRoute)
			key := hostsKey(httpRoute.Hostnames)
			if found, ok := merged[key]; ok {
				found.Name = gateway.ID.Service + "-merged-" + key
				found.Namespace = gateway.ID.ConsulNamespace
				found.Rules = append(found.Rules, httpRoute.Rules...)
				sort.SliceStable(found.Rules, func(i, j int) bool {
					return compareHTTPRules(found.Rules[i], found.Rules[j])
				})
				merged[key] = found
			} else {
				merged[key] = httpRoute
			}
		default:
			unmerged = append(unmerged, route)
		}
	}

	for _, route := range merged {
		unmerged = append(unmerged, route)
	}

	return unmerged
}

func hostsKey(hosts []string) string {
	sort.Strings(hosts)
	hostsHash := crc32.NewIEEE()
	for _, h := range hosts {
		if _, err := hostsHash.Write([]byte(h)); err != nil {
			continue
		}
	}
	return strconv.FormatUint(uint64(hostsHash.Sum32()), 16)
}

func compareHTTPRules(ruleA, ruleB core.HTTPRouteRule) bool {
	matchesA := ruleA.Matches
	matchesB := ruleB.Matches

	// this tries to implement some of the logic specified by the K8S gateway API spec

	// Proxy or Load Balancer routing configuration generated from HTTPRoutes MUST prioritize
	// rules based on the following criteria, continuing on ties. Precedence must be given
	// to the the Rule with the largest number of:
	// Characters in a matching non-wildcard hostname.
	// Characters in a matching hostname.
	// Characters in a matching path.
	// Header matches.
	// Query param matches.

	var longestPathMatchA int
	for _, match := range matchesA {
		pathLength := len(match.Path.Value)
		if longestPathMatchA < pathLength {
			longestPathMatchA = pathLength
		}
	}
	var longestPathMatchB int
	for _, match := range matchesB {
		pathLength := len(match.Path.Value)
		if longestPathMatchB < pathLength {
			longestPathMatchB = pathLength
		}
	}
	return longestPathMatchA > longestPathMatchB
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
				routers.Add(router)
				splitters.Merge(splits)
				defaults.Merge(serviceDefaults)
			}
		}

		if len(services) > 0 {
			tls := &api.GatewayTLSConfig{}
			if len(listener.Certificates) > 0 {
				tls.SDS = &api.GatewayTLSSDSConfig{
					ClusterName:  "sds-cluster",
					CertResource: listener.Certificates[0],
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

func (a *ConsulSyncAdapter) entriesForGateway(id core.GatewayID) (*consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	existing, found := a.sync[id]
	if !found {
		routers := consul.NewConfigEntryIndex(api.ServiceRouter)
		splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
		defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)
		return routers, splitters, defaults
	}
	return existing.routers, existing.splitters, existing.defaults
}

func (a *ConsulSyncAdapter) setEntriesForGateway(gateway core.ResolvedGateway, routers *consul.ConfigEntryIndex, splitters *consul.ConfigEntryIndex, defaults *consul.ConfigEntryIndex) {
	a.sync[gateway.ID] = syncState{
		routers:   routers,
		splitters: splitters,
		defaults:  defaults,
	}
}

func (a *ConsulSyncAdapter) syncIntentionsForGateway(gateway core.GatewayID, ingress *api.IngressGatewayConfigEntry) error {
	if a.intentions[gateway] == nil {
		a.intentions[gateway] = consul.NewIntentionsReconciler(a.consul, ingress, a.logger)
	} else {
		a.intentions[gateway].SetIngressServices(ingress)
	}
	return a.intentions[gateway].Reconcile()
}

func (a *ConsulSyncAdapter) stopIntentionSyncForGateway(gw core.GatewayID) {
	if ir, ok := a.intentions[gw]; ok {
		ir.Stop()
		delete(a.intentions, gw)
	}
}

func (a *ConsulSyncAdapter) Clear(ctx context.Context, id core.GatewayID) error {
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

	if err := a.deleteConfigEntries(ctx, removedRouters...); err != nil {
		return fmt.Errorf("error removing service router config entries: %w", err)
	}
	if err := a.deleteConfigEntries(ctx, removedSplitters...); err != nil {
		return fmt.Errorf("error removing service splitter config entries: %w", err)
	}
	if err := a.deleteConfigEntries(ctx, removedDefaults...); err != nil {
		return fmt.Errorf("error removing service defaults config entries: %w", err)
	}

	if err := a.deleteConfigEntries(ctx, ingress); err != nil {
		return fmt.Errorf("error removing ingress config entry: %w", err)
	}

	a.stopIntentionSyncForGateway(id)
	delete(a.sync, id)
	return nil
}

func (a *ConsulSyncAdapter) Sync(ctx context.Context, gateway core.ResolvedGateway) error {
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
		return fmt.Errorf("error adding service defaults config entries: %w", err)
	}
	if err := a.setConfigEntries(ctx, addedRouters...); err != nil {
		return fmt.Errorf("error adding service router config entries: %w", err)
	}
	if err := a.setConfigEntries(ctx, addedSplitters...); err != nil {
		return fmt.Errorf("error adding service splitter config entries: %w", err)
	}

	if err := a.setConfigEntries(ctx, ingress); err != nil {
		return fmt.Errorf("error adding ingress config entry: %w", err)
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

	a.setEntriesForGateway(gateway, computedRouters, computedSplitters, computedDefaults)
	if err := a.syncIntentionsForGateway(gateway.ID, ingress); err != nil {
		return fmt.Errorf("error syncing service intention config entries: %w", err)
	}

	return nil
}
