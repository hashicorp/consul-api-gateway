package consul

import (
	"fmt"

	"github.com/hashicorp/consul/api"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// HTTPRouteToServiceDiscoChain will convert a k8s HTTPRoute to a Consul service-router config entry and 0 or
// more service-splitter config entries. A prefix can be given to prefix all config entry names with.
func HTTPRouteToServiceDiscoChain(route *gw.HTTPRoute, prefix string, meta map[string]string) (*api.ServiceRouterConfigEntry, []*api.ServiceSplitterConfigEntry) {
	var router *api.ServiceRouterConfigEntry
	routeName := fmt.Sprintf("%s%s", prefix, route.Name)
	router = &api.ServiceRouterConfigEntry{
		Kind: api.ServiceRouter,
		Name: routeName,
		Meta: meta,
	}
	var splitters []*api.ServiceSplitterConfigEntry

	// All route rules are enumerated and a ServiceRoute created for each.
	for idx, rule := range route.Spec.Rules {
		var destService string
		// If the rule only has 1 ForwardTo target defined a splitter does not need to be created and the
		// ServiceRoute.Destination can be set to the ForwardTo service name
		if len(rule.BackendRefs) == 1 {
			destService = rule.BackendRefs[0].Name
		} else {
			destService = fmt.Sprintf("%s-%d", routeName, idx)
			splitter := &api.ServiceSplitterConfigEntry{
				Kind:   api.ServiceSplitter,
				Name:   destService,
				Splits: []api.ServiceSplit{},
				Meta:   meta,
			}

			for _, forward := range rule.BackendRefs {
				// if a forward rule does not define a weight it is defaulted to 1
				split := api.ServiceSplit{
					Weight: float32(1),
				}
				if forward.Weight != nil {
					split.Weight = float32(*forward.Weight)
				}

				// The gateway api spec states that a weight of 0 must not be routed to, thus skip this split
				if split.Weight == 0 {
					continue
				}
				split.Service = forward.Name
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
					Service: destService,
				},
			})
		}
		for _, match := range rule.Matches {
			router.Routes = append(router.Routes, api.ServiceRoute{
				Match: &api.ServiceRouteMatch{HTTP: HTTPRouteMatchToServiceRouteHTTPMatch(match)},
				Destination: &api.ServiceRouteDestination{
					Service: destService,
				},
			})
		}
	}

	return router, splitters
}

func HTTPRouteMatchToServiceRouteHTTPMatch(route gw.HTTPRouteMatch) *api.ServiceRouteHTTPMatch {
	var match api.ServiceRouteHTTPMatch
	if route.Path != nil && route.Path.Type != nil && route.Path.Value != nil {
		switch *route.Path.Type {
		case gw.PathMatchExact:
			match.PathExact = *route.Path.Value
		case gw.PathMatchPrefix:
			match.PathPrefix = *route.Path.Value
		case gw.PathMatchRegularExpression:
			match.PathRegex = *route.Path.Value
		}
		if *route.Path.Type == gw.PathMatchExact {
			match.PathExact = *route.Path.Value
		}
	}

	for _, header := range route.Headers {
		if header.Type == nil {
			t := gw.HeaderMatchExact
			header.Type = &t
		}
		switch *header.Type {
		case gw.HeaderMatchExact:
			match.Header = append(match.Header, api.ServiceRouteHTTPMatchHeader{
				Name:  header.Name,
				Exact: header.Value,
			})
		}
	}

	for _, param := range route.QueryParams {
		if param.Type == nil {
			t := gw.QueryParamMatchExact
			param.Type = &t
		}
		switch *param.Type {
		case gw.QueryParamMatchExact:
			match.QueryParam = append(match.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
				Name:  param.Name,
				Exact: param.Value,
			})
		}
	}

	return &match
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
