package consul

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
)

// httpRouteDiscoveryChain will convert a k8s HTTPRoute to a Consul service-router config entry and 0 or
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

// httpRouteFiltersToServiceRouteHeaderModifier will consolidate a list of HTTP filters
// into a single set of header modifications for Consul to make as a request passes through.
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
			// is all implementation-specific behavior and undefined by the spec.
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

// compareHTTPRules implements the non-hostname order of precedence for routes specified by the K8s Gateway API spec.
// https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPRouteRule
//
// Ordering prefers matches based on the largest number of:
//
//   1. characters in a matching non-wildcard hostname
//   2. characters in a matching hostname
//   3. characters in a matching path
//   4. header matches
//   5. query param matches
//
// The hostname-specific comparison (1+2) occur in Envoy outside of our control:
// https://github.com/envoyproxy/envoy/blob/5c4d4bd957f9402eca80bef82e7cc3ae714e04b4/source/common/router/config_impl.cc#L1645-L1682
func compareHTTPRules(ruleA, ruleB core.HTTPMatch) bool {
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

type hostnameMatch struct {
	match    core.HTTPMatch
	filters  []core.HTTPFilter
	services []core.HTTPService
}

type routeConsolidator struct {
	matchesByHostname map[string][]hostnameMatch
}

func newRouteConsolidator() *routeConsolidator {
	return &routeConsolidator{
		matchesByHostname: map[string][]hostnameMatch{},
	}
}

// add takes a new route and flattens its rule matches out per hostname.
// This is required since a single route can specify multiple hostnames, and a
// single hostname can be specified in multiple routes. Routing for a given
// hostname must behave based on the aggregate of all rules that apply to it.
func (f *routeConsolidator) add(route core.HTTPRoute) {
	for _, host := range route.Hostnames {
		matches, ok := f.matchesByHostname[host]
		if !ok {
			matches = []hostnameMatch{}
		}

		for _, rule := range route.Rules {
			// If a rule has no matches defined, add default match
			if len(rule.Matches) == 0 {
				rule.Matches = []core.HTTPMatch{{
					Path: core.HTTPPathMatch{
						Type:  core.HTTPPathMatchPrefixType,
						Value: "/",
					},
				}}
			}

			// Add all matches for this rule to the list for this hostname
			for _, match := range rule.Matches {
				matches = append(matches, hostnameMatch{
					match:    match,
					filters:  rule.Filters,
					services: rule.Services,
				})
			}
		}

		f.matchesByHostname[host] = matches
	}
}

// consolidate combines all rules into the shortest possible list of routes
// with one route per hostname containing all rules for that hostname.
func (f *routeConsolidator) consolidate(gateway core.ResolvedGateway) []core.HTTPRoute {
	var routes []core.HTTPRoute

	for hostname, rules := range f.matchesByHostname {
		// Create route for this hostname
		route := core.HTTPRoute{
			CommonRoute: core.CommonRoute{
				Name:      gateway.ID.Service + "-" + hostsKey(hostname),
				Namespace: gateway.ID.ConsulNamespace,
				Meta:      gateway.Meta,
			},
			Hostnames: []string{hostname},
			Rules:     make([]core.HTTPRouteRule, 0, len(rules)),
		}

		// Sort rules for this hostname in order of precedence
		sort.SliceStable(rules, func(i, j int) bool {
			return compareHTTPRules(rules[i].match, rules[j].match)
		})

		// Add all rules for this hostname
		for _, rule := range rules {
			route.Rules = append(route.Rules, core.HTTPRouteRule{
				Matches:  []core.HTTPMatch{rule.match},
				Filters:  rule.filters,
				Services: rule.services,
			})
		}

		routes = append(routes, route)
	}

	return routes
}
