package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/state"
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

	sync  map[state.GatewayID]syncState
	mutex sync.Mutex
}

func NewConsulSyncAdapter(logger hclog.Logger, consul *api.Client) *ConsulSyncAdapter {
	return &ConsulSyncAdapter{
		logger: logger,
		consul: consul,
		sync:   make(map[state.GatewayID]syncState),
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
func httpRouteDiscoveryChain(route state.HTTPRoute) (*api.ServiceRouterConfigEntry, []*api.ServiceSplitterConfigEntry) {
	router := &api.ServiceRouterConfigEntry{
		Kind: api.ServiceRouter,
		Name: route.Name(),
		Meta: route.Meta(),
	}
	var splitters []*api.ServiceSplitterConfigEntry

	for idx, rule := range route.Rules {
		var destination state.ResolvedService
		if len(rule.Services) == 1 {
			destination = rule.Services[0].Service
		} else {
			// create a virtual service to split
			destination = state.ResolvedService{
				Service: fmt.Sprintf("%s-%d", route.Name(), idx),
			}
			splitter := &api.ServiceSplitterConfigEntry{
				Kind:      api.ServiceSplitter,
				Name:      destination.Service,
				Namespace: destination.ConsulNamespace,
				Splits:    []api.ServiceSplit{},
				Meta:      route.Meta(),
			}

			totalWeight := int32(0)
			for _, service := range rule.Services {
				totalWeight += service.Weight
			}

			for _, service := range rule.Services {
				if service.Weight == 0 {
					continue
				}
				split := api.ServiceSplit{
					Weight: float32(service.Weight) / float32(totalWeight),
				}
				split.Service = service.Service.Service
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
					Service:   destination.Service,
					Namespace: destination.ConsulNamespace,
				},
			})
		}
		for _, match := range rule.Matches {
			router.Routes = append(router.Routes, api.ServiceRoute{
				Match: &api.ServiceRouteMatch{HTTP: httpRouteMatchToServiceRouteHTTPMatch(match)},
				Destination: &api.ServiceRouteDestination{
					Service:   destination.Service,
					Namespace: destination.ConsulNamespace,
				},
			})
		}
	}

	return router, splitters
}

func httpRouteMatchToServiceRouteHTTPMatch(match state.HTTPMatch) *api.ServiceRouteHTTPMatch {
	var consulMatch api.ServiceRouteHTTPMatch
	switch match.Path.Type {
	case state.HTTPPathMatchExactType:
		consulMatch.PathExact = match.Path.Value
	case state.HTTPPathMatchPrefixType:
		consulMatch.PathPrefix = match.Path.Value
	case state.HTTPPathMatchRegularExpressionType:
		consulMatch.PathRegex = match.Path.Value
	}

	for _, header := range match.Headers {
		switch header.Type {
		case state.HTTPHeaderMatchExactType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:  header.Name,
				Exact: header.Value,
			})
		case state.HTTPHeaderMatchPrefixType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:   header.Name,
				Prefix: header.Value,
			})
		case state.HTTPHeaderMatchSuffixType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:   header.Name,
				Suffix: header.Value,
			})
		case state.HTTPHeaderMatchPresentType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:    header.Name,
				Present: true,
			})
		case state.HTTPHeaderMatchRegularExpressionType:
			consulMatch.Header = append(consulMatch.Header, api.ServiceRouteHTTPMatchHeader{
				Name:  header.Name,
				Regex: header.Value,
			})
		}
	}

	for _, query := range match.Query {
		switch query.Type {
		case state.HTTPQueryMatchExactType:
			consulMatch.QueryParam = append(consulMatch.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:  query.Name,
				Exact: query.Value,
			})
		case state.HTTPQueryMatchPresentType:
			consulMatch.QueryParam = append(consulMatch.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:    query.Name,
				Present: true,
			})
		case state.HTTPQueryMatchRegularExpressionType:
			consulMatch.QueryParam = append(consulMatch.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:  query.Name,
				Regex: query.Value,
			})
		}
	}

	switch match.Method {
	case state.HTTPMethodConnect:
		consulMatch.Methods = append(consulMatch.Methods, "CONNECT")
	case state.HTTPMethodDelete:
		consulMatch.Methods = append(consulMatch.Methods, "DELETE")
	case state.HTTPMethodGet:
		consulMatch.Methods = append(consulMatch.Methods, "GET")
	case state.HTTPMethodHead:
		consulMatch.Methods = append(consulMatch.Methods, "HEAD")
	case state.HTTPMethodOptions:
		consulMatch.Methods = append(consulMatch.Methods, "OPTIONS")
	case state.HTTPMethodPatch:
		consulMatch.Methods = append(consulMatch.Methods, "PATCH")
	case state.HTTPMethodPost:
		consulMatch.Methods = append(consulMatch.Methods, "POST")
	case state.HTTPMethodPut:
		consulMatch.Methods = append(consulMatch.Methods, "PUT")
	case state.HTTPMethodTrace:
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

func routeDiscoveryChain(route state.ResolvedRoute) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	meta := route.Meta()
	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	switch route.Type() {
	case state.ResolvedHTTPRouteType:
		httpRoute := route.(state.HTTPRoute)
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
			Namespace: httpRoute.Namespace(),
		}, router, splitters, defaults
	default:
		return nil, nil, nil, nil
	}
}

func discoveryChain(gateway state.ResolvedGateway) (*api.IngressGatewayConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
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

		for _, route := range listener.Routes {
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

func (a *ConsulSyncAdapter) entriesForGateway(gateway state.ResolvedGateway) (*consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	existing, found := a.sync[gateway.ID]
	if !found {
		routers := consul.NewConfigEntryIndex(api.ServiceRouter)
		splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
		defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)
		return routers, splitters, defaults
	}
	return existing.routers, existing.splitters, existing.defaults
}

func (a *ConsulSyncAdapter) setEntriesForGateway(gateway state.ResolvedGateway, routers *consul.ConfigEntryIndex, splitters *consul.ConfigEntryIndex, defaults *consul.ConfigEntryIndex) {
	a.sync[gateway.ID] = syncState{
		routers:   routers,
		splitters: splitters,
		defaults:  defaults,
	}
}

func (a *ConsulSyncAdapter) Sync(ctx context.Context, gateway state.ResolvedGateway) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.logger.IsTrace() {
		started := time.Now()
		a.logger.Trace("started reconciliation", "time", started)
		defer a.logger.Trace("reconciliation finished", "time", time.Now(), "spent", time.Since(started))
	}

	ingress, computedRouters, computedSplitters, computedDefaults := discoveryChain(gateway)
	existingRouters, existingSplitters, existingDefaults := a.entriesForGateway(gateway)

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

	return nil
}
