package consul

import (
	"fmt"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"github.com/hashicorp/consul/api"
)

func GatewayToIngress(gateway *gw.Gateway) *api.IngressGatewayConfigEntry {
	return &api.IngressGatewayConfigEntry{
		Kind:      api.IngressGateway,
		Name:      "polar-" + gateway.Name,
		TLS:       api.GatewayTLSConfig{},
		Listeners: []api.IngressListener{},
		Meta:      map[string]string{},
	}
}

func HTTPRouteToServiceDiscoChain(route *gw.HTTPRoute, prefix string) (*api.ServiceRouterConfigEntry, []*api.ServiceSplitterConfigEntry) {
	var router *api.ServiceRouterConfigEntry
	routeName := fmt.Sprintf("%s%s", prefix, route.Name)
	router = &api.ServiceRouterConfigEntry{
		Kind:   api.ServiceRouter,
		Name:   routeName,
		Routes: []api.ServiceRoute{},
	}
	splitters := []*api.ServiceSplitterConfigEntry{}
	for idx, rule := range route.Spec.Rules {
		var destService string
		if len(rule.ForwardTo) == 1 && rule.ForwardTo[0].ServiceName != nil {
			destService = *rule.ForwardTo[0].ServiceName
		} else {
			destService = fmt.Sprintf("%s-%d", routeName, idx)
			splitter := &api.ServiceSplitterConfigEntry{
				Kind:   api.ServiceSplitter,
				Name:   destService,
				Splits: []api.ServiceSplit{},
			}
			for _, forward := range rule.ForwardTo {
				split := api.ServiceSplit{}
				split.Weight = float32(1)
				if forward.Weight != nil {
					split.Weight = float32(*forward.Weight)
				}
				if split.Weight == 0 {
					continue
				}
				if forward.ServiceName != nil {
					split.Service = *forward.ServiceName
				}
				splitter.Splits = append(splitter.Splits, split)
			}
			if len(splitter.Splits) > 0 {
				splitters = append(splitters, splitter)
			}
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
	match := &api.ServiceRouteHTTPMatch{
		Header:     []api.ServiceRouteHTTPMatchHeader{},
		QueryParam: []api.ServiceRouteHTTPMatchQueryParam{},
	}
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

	if route.Headers != nil && route.Headers.Type != nil && route.Headers.Values != nil {
		for header, value := range route.Headers.Values {
			switch *route.Headers.Type {
			case gw.HeaderMatchExact:
				match.Header = append(match.Header, api.ServiceRouteHTTPMatchHeader{
					Name:  header,
					Exact: value,
				})
			}
		}
	}

	if route.QueryParams != nil && route.QueryParams.Type != nil && route.QueryParams.Values != nil {
		for param, value := range route.QueryParams.Values {
			switch *route.QueryParams.Type {
			case gw.QueryParamMatchExact:
				match.QueryParam = append(match.QueryParam, api.ServiceRouteHTTPMatchQueryParam{
					Name:  param,
					Exact: value,
				})
			}
		}
	}

	return match
}

func httpServiceDefault(entry api.ConfigEntry) *api.ServiceConfigEntry {
	return &api.ServiceConfigEntry{
		Kind:      api.ServiceDefaults,
		Name:      entry.GetName(),
		Namespace: entry.GetNamespace(),
		Protocol:  "http",
	}
}
