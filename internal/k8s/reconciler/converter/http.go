// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package converter

import (
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

type HTTPRouteConverter struct {
	namespace string
	hostname  string
	prefix    string
	meta      map[string]string
	route     *gwv1alpha2.HTTPRoute
	state     *state.RouteState
}

type HTTPRouteConverterConfig struct {
	Namespace string
	Hostname  string
	Prefix    string
	Meta      map[string]string
	Route     *gwv1alpha2.HTTPRoute
	State     *state.RouteState
}

func NewHTTPRouteConverter(config HTTPRouteConverterConfig) *HTTPRouteConverter {
	return &HTTPRouteConverter{
		namespace: config.Namespace,
		hostname:  config.Hostname,
		prefix:    config.Prefix,
		meta:      config.Meta,
		route:     config.Route,
		state:     config.State,
	}
}

func (c *HTTPRouteConverter) Convert() core.ResolvedRoute {
	hostnames := []string{}
	for _, hostname := range c.route.Spec.Hostnames {
		hostnames = append(hostnames, string(hostname))
	}
	if len(hostnames) == 0 {
		if c.hostname == "" {
			c.hostname = "*"
		}
		hostnames = append(hostnames, c.hostname)
	}
	name := c.prefix + c.route.Name

	return core.NewHTTPRouteBuilder().
		WithName(name).
		// we always use the listener namespace for the routes
		// themselves, while the services they route to might
		// be in different namespaces
		WithNamespace(c.namespace).
		WithHostnames(hostnames).
		WithMeta(c.meta).
		WithRules(httpReferencesToRules(c.state.References)).
		Build()
}

var methodMappings = map[gwv1alpha2.HTTPMethod]core.HTTPMethod{
	gwv1alpha2.HTTPMethodConnect: core.HTTPMethodConnect,
	gwv1alpha2.HTTPMethodDelete:  core.HTTPMethodDelete,
	gwv1alpha2.HTTPMethodPost:    core.HTTPMethodPost,
	gwv1alpha2.HTTPMethodPut:     core.HTTPMethodPut,
	gwv1alpha2.HTTPMethodPatch:   core.HTTPMethodPatch,
	gwv1alpha2.HTTPMethodGet:     core.HTTPMethodGet,
	gwv1alpha2.HTTPMethodOptions: core.HTTPMethodOptions,
	gwv1alpha2.HTTPMethodTrace:   core.HTTPMethodTrace,
	gwv1alpha2.HTTPMethodHead:    core.HTTPMethodHead,
}

var pathMappings = map[gwv1alpha2.PathMatchType]core.HTTPPathMatchType{
	gwv1alpha2.PathMatchExact:             core.HTTPPathMatchExactType,
	gwv1alpha2.PathMatchPathPrefix:        core.HTTPPathMatchPrefixType,
	gwv1alpha2.PathMatchRegularExpression: core.HTTPPathMatchRegularExpressionType,
}

var queryMappings = map[gwv1alpha2.QueryParamMatchType]core.HTTPQueryMatchType{
	gwv1alpha2.QueryParamMatchExact:             core.HTTPQueryMatchExactType,
	gwv1alpha2.QueryParamMatchRegularExpression: core.HTTPQueryMatchRegularExpressionType,
}

var headerMappings = map[gwv1alpha2.HeaderMatchType]core.HTTPHeaderMatchType{
	gwv1alpha2.HeaderMatchExact:             core.HTTPHeaderMatchExactType,
	gwv1alpha2.HeaderMatchRegularExpression: core.HTTPHeaderMatchRegularExpressionType,
}

func httpReferencesToRules(referenceMap service.RouteRuleReferenceMap) []core.HTTPRouteRule {
	resolved := []core.HTTPRouteRule{}

	for rule, references := range referenceMap {
		filters := convertHTTPRouteFilters(rule.HTTPRule.Filters)
		matches := []core.HTTPMatch{}
		for _, match := range rule.HTTPRule.Matches {
			stateMatch := core.HTTPMatch{}
			if match.Method != nil {
				if method, found := methodMappings[*match.Method]; found {
					stateMatch.Method = method
				}
			}
			if match.Path != nil && match.Path.Value != nil {
				matchType := gwv1alpha2.PathMatchExact
				if match.Path.Type != nil {
					matchType = *match.Path.Type
				}
				if mappedType, found := pathMappings[matchType]; found {
					stateMatch.Path = core.HTTPPathMatch{
						Type:  mappedType,
						Value: *match.Path.Value,
					}
				}
			}
			for _, param := range match.QueryParams {
				matchType := gwv1alpha2.QueryParamMatchExact
				if param.Type != nil {
					matchType = *param.Type
				}
				if mappedType, found := queryMappings[matchType]; found {
					stateMatch.Query = append(stateMatch.Query, core.HTTPQueryMatch{
						Type:  mappedType,
						Name:  param.Name,
						Value: param.Value,
					})
				}
			}
			for _, header := range match.Headers {
				matchType := gwv1alpha2.HeaderMatchExact
				if header.Type != nil {
					matchType = *header.Type
				}
				if mappedType, found := headerMappings[matchType]; found {
					stateMatch.Headers = append(stateMatch.Headers, core.HTTPHeaderMatch{
						Type:  mappedType,
						Name:  string(header.Name),
						Value: header.Value,
					})
				}
			}
			matches = append(matches, stateMatch)
		}

		services := []core.HTTPService{}
		for _, reference := range references {
			switch reference.Type {
			case service.ConsulServiceReference:
				weight := int32(1)
				if reference.Reference.HTTPRef.Weight != nil {
					weight = *reference.Reference.HTTPRef.Weight
				}
				// note that we ignore the Port value
				services = append(services, core.HTTPService{
					Service: core.ResolvedService{
						ConsulNamespace: reference.Consul.Namespace,
						Service:         reference.Consul.Name,
					},
					Weight:  weight,
					Filters: convertHTTPRouteFilters(reference.Reference.HTTPRef.Filters),
				})
			default:
				continue
			}
		}
		resolved = append(resolved, core.HTTPRouteRule{
			Filters:  filters,
			Matches:  matches,
			Services: services,
		})
	}
	return resolved
}

func convertHTTPRouteFilters(routeFilters []gwv1alpha2.HTTPRouteFilter) []core.HTTPFilter {
	filters := []core.HTTPFilter{}
	for _, filter := range routeFilters {
		switch filter.Type {
		case gwv1alpha2.HTTPRouteFilterRequestHeaderModifier:
			filters = append(filters, core.HTTPFilter{
				Type: core.HTTPHeaderFilterType,
				Header: core.HTTPHeaderFilter{
					Set:    httpHeadersToMap(filter.RequestHeaderModifier.Set),
					Add:    httpHeadersToMap(filter.RequestHeaderModifier.Add),
					Remove: filter.RequestHeaderModifier.Remove,
				},
			})
		case gwv1alpha2.HTTPRouteFilterRequestRedirect:
			scheme := ""
			if filter.RequestRedirect.Scheme != nil {
				scheme = *filter.RequestRedirect.Scheme
			}
			hostname := ""
			if filter.RequestRedirect.Hostname != nil {
				hostname = string(*filter.RequestRedirect.Hostname)
			}
			port := 0
			if filter.RequestRedirect.Port != nil {
				port = int(*filter.RequestRedirect.Port)
			}
			statusCode := 0
			if filter.RequestRedirect.StatusCode != nil {
				statusCode = *filter.RequestRedirect.StatusCode
			}
			filters = append(filters, core.HTTPFilter{
				Type: core.HTTPRedirectFilterType,
				Redirect: core.HTTPRedirectFilter{
					Scheme:   scheme,
					Hostname: hostname,
					Port:     port,
					Status:   statusCode,
				},
			})
		case gwv1alpha2.HTTPRouteFilterURLRewrite:
			// We currently only support prefix match replacement
			if filter.URLRewrite.Path == nil ||
				filter.URLRewrite.Path.Type != gwv1alpha2.PrefixMatchHTTPPathModifier ||
				filter.URLRewrite.Path.ReplacePrefixMatch == nil {
				continue
			}

			filters = append(filters, core.HTTPFilter{
				Type: core.HTTPURLRewriteFilterType,
				URLRewrite: core.HTTPURLRewriteFilter{
					Type:               core.URLRewriteReplacePrefixMatchType,
					ReplacePrefixMatch: *filter.URLRewrite.Path.ReplacePrefixMatch,
				},
			})
		}
	}
	return filters
}

func httpHeadersToMap(headers []gwv1alpha2.HTTPHeader) map[string]string {
	resolved := make(map[string]string)
	for _, header := range headers {
		resolved[string(header.Name)] = header.Value
	}
	return resolved
}
