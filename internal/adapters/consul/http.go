package consul

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
)

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

func hostsKey(hosts ...string) string {
	sort.Strings(hosts)
	hostsHash := crc32.NewIEEE()
	for _, h := range hosts {
		if _, err := hostsHash.Write([]byte(h)); err != nil {
			continue
		}
	}
	return strconv.FormatUint(uint64(hostsHash.Sum32()), 16)
}

func compareHTTPRules(ruleA, ruleB core.HTTPMatch) bool {
	// this tries to implement some of the logic specified by the K8S gateway API spec

	// Proxy or Load Balancer routing configuration generated from HTTPRoutes MUST prioritize
	// rules based on the following criteria, continuing on ties. Precedence must be given
	// to the the Rule with the largest number of:
	// Characters in a matching non-wildcard hostname.
	// Characters in a matching hostname.
	// Characters in a matching path.
	// Header matches.
	// Query param matches.

	if len(ruleA.Path.Value) != len(ruleB.Path.Value) {
		return len(ruleA.Path.Value) > len(ruleB.Path.Value)
	}
	if len(ruleA.Headers) != len(ruleB.Headers) {
		return len(ruleA.Headers) > len(ruleB.Headers)
	}
	return len(ruleA.Query) > len(ruleB.Query)
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

type flattenedRoute struct {
	match    core.HTTPMatch
	filters  []core.HTTPFilter
	services []core.HTTPService
}

type flattenedRouteMap struct {
	flattenedRoutesByHostname map[string][]flattenedRoute
}

func newflattenedRouteMap() *flattenedRouteMap {
	return &flattenedRouteMap{
		flattenedRoutesByHostname: map[string][]flattenedRoute{},
	}
}

func (f *flattenedRouteMap) flatten(route core.HTTPRoute) {
	for _, host := range route.Hostnames {
		found, ok := f.flattenedRoutesByHostname[host]
		if !ok {
			found = []flattenedRoute{}
		}
		for _, rule := range route.Rules {
			if len(rule.Matches) == 0 {
				rule.Matches = []core.HTTPMatch{{
					Path: core.HTTPPathMatch{
						Type:  core.HTTPPathMatchPrefixType,
						Value: "/",
					},
				}}
			}
			for _, match := range rule.Matches {
				found = append(found, flattenedRoute{
					match:    match,
					filters:  rule.Filters,
					services: rule.Services,
				})
			}
		}
		f.flattenedRoutesByHostname[host] = found
	}
}

func (f *flattenedRouteMap) constructRoutes(gateway core.ResolvedGateway) []core.HTTPRoute {
	coreRoutes := []core.HTTPRoute{}
	for hostname, routes := range f.flattenedRoutesByHostname {
		sort.SliceStable(routes, func(i, j int) bool {
			return compareHTTPRules(routes[i].match, routes[j].match)
		})
		rules := []core.HTTPRouteRule{}
		for _, match := range routes {
			rules = append(rules, core.HTTPRouteRule{
				Matches:  []core.HTTPMatch{match.match},
				Filters:  match.filters,
				Services: match.services,
			})
		}
		name := gateway.ID.Service + "-" + hostsKey(hostname)
		coreRoutes = append(coreRoutes, core.HTTPRoute{
			CommonRoute: core.CommonRoute{
				Name:      name,
				Namespace: gateway.ID.ConsulNamespace,
				Meta:      gateway.Meta,
			},
			Hostnames: []string{hostname},
			Rules:     rules,
		})
	}
	return coreRoutes
}
