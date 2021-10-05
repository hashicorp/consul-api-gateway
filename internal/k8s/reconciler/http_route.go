package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/state"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func HTTPRouteID(namespacedName types.NamespacedName) string {
	return "http-" + namespacedName.String()
}

func convertHTTPRoute(hostname, prefix string, meta map[string]string, route *gw.HTTPRoute, k8sRoute *K8sRoute) *state.ResolvedRoute {
	hostnames := []string{}
	for _, hostname := range route.Spec.Hostnames {
		hostnames = append(hostnames, string(hostname))
	}
	if len(hostnames) == 0 {
		hostnames = append(hostnames, hostname)
	}
	name := prefix + route.Name

	// TODO: add consul namespace mappings
	resolved := state.NewHTTPRouteBuilder().
		WithName(name).
		WithHostnames(hostnames).
		WithMeta(meta).
		WithRules(httpReferencesToRules(k8sRoute.references)).
		Build()
	return &resolved
}

var methodMappings = map[gw.HTTPMethod]state.HTTPMethod{
	gw.HTTPMethodConnect: state.HTTPMethodConnect,
	gw.HTTPMethodDelete:  state.HTTPMethodDelete,
	gw.HTTPMethodPost:    state.HTTPMethodPost,
	gw.HTTPMethodPut:     state.HTTPMethodPut,
	gw.HTTPMethodPatch:   state.HTTPMethodPatch,
	gw.HTTPMethodGet:     state.HTTPMethodGet,
	gw.HTTPMethodOptions: state.HTTPMethodOptions,
	gw.HTTPMethodTrace:   state.HTTPMethodTrace,
	gw.HTTPMethodHead:    state.HTTPMethodHead,
}

var pathMappings = map[gw.PathMatchType]state.HTTPPathMatchType{
	gw.PathMatchExact:             state.HTTPPathMatchExactType,
	gw.PathMatchRegularExpression: state.HTTPPathMatchRegularExpressionType,
}

var queryMappings = map[gw.QueryParamMatchType]state.HTTPQueryMatchType{
	gw.QueryParamMatchExact: state.HTTPQueryMatchExactType,
}

var headerMappings = map[gw.HeaderMatchType]state.HTTPHeaderMatchType{
	gw.HeaderMatchExact:             state.HTTPHeaderMatchExactType,
	gw.HeaderMatchRegularExpression: state.HTTPHeaderMatchRegularExpressionType,
}

func httpReferencesToRules(referenceMap service.RouteRuleReferenceMap) []state.HTTPRouteRule {
	resolved := []state.HTTPRouteRule{}

	for rule, references := range referenceMap {
		filters := convertHTTPRouteFilters(rule.HTTPRule.Filters)
		matches := []state.HTTPMatch{}
		for _, match := range rule.HTTPRule.Matches {
			stateMatch := state.HTTPMatch{}
			if match.Method != nil {
				if method, found := methodMappings[*match.Method]; found {
					stateMatch.Method = method
				}
			}
			if match.Path != nil && match.Path.Value != nil {
				matchType := gw.PathMatchExact
				if match.Path.Type != nil {
					matchType = *match.Path.Type
				}
				if mappedType, found := pathMappings[matchType]; found {
					stateMatch.Path = state.HTTPPathMatch{
						Type:  mappedType,
						Value: *match.Path.Value,
					}
				}
			}
			for _, param := range match.QueryParams {
				matchType := gw.QueryParamMatchExact
				if param.Type != nil {
					matchType = *param.Type
				}
				if mappedType, found := queryMappings[matchType]; found {
					stateMatch.Query = append(stateMatch.Query, state.HTTPQueryMatch{
						Type:  mappedType,
						Name:  param.Name,
						Value: param.Value,
					})
				}
			}
			for _, header := range match.Headers {
				matchType := gw.HeaderMatchExact
				if header.Type != nil {
					matchType = *header.Type
				}
				if mappedType, found := headerMappings[matchType]; found {
					stateMatch.Headers = append(stateMatch.Headers, state.HTTPHeaderMatch{
						Type:  mappedType,
						Name:  string(header.Name),
						Value: header.Value,
					})
				}
			}
			matches = append(matches, stateMatch)
		}

		services := []state.HTTPService{}
		for _, reference := range references {
			switch reference.Type {
			case service.ConsulServiceReference:
				weight := int32(1)
				if reference.Reference.HTTPRef.Weight != nil {
					weight = *reference.Reference.HTTPRef.Weight
				}
				// note that we ignore the Port value
				services = append(services, state.HTTPService{
					Service: state.ResolvedService{
						ConsulNamespace: reference.Consul.Namespace,
						Service:         reference.Consul.Name,
					},
					Weight:  weight,
					Filters: convertHTTPRouteFilters(reference.Reference.HTTPRef.Filters),
				})
			default:
				// TODO: support other reference types
				continue
			}
		}
		resolved = append(resolved, state.HTTPRouteRule{
			Filters:  filters,
			Matches:  matches,
			Services: services,
		})
	}
	return resolved
}

func convertHTTPRouteFilters(routeFilters []gw.HTTPRouteFilter) []state.HTTPFilter {
	filters := []state.HTTPFilter{}
	for _, filter := range routeFilters {
		switch filter.Type {
		case gw.HTTPRouteFilterRequestHeaderModifier:
			filters = append(filters, state.HTTPFilter{
				Type: state.HTTPHeaderFilterType,
				Header: state.HTTPHeaderFilter{
					Set:    httpHeadersToMap(filter.RequestHeaderModifier.Set),
					Add:    httpHeadersToMap(filter.RequestHeaderModifier.Add),
					Remove: filter.RequestHeaderModifier.Remove,
				},
			})
		case gw.HTTPRouteFilterRequestRedirect:
			scheme := ""
			if filter.RequestRedirect.Protocol != nil {
				scheme = *filter.RequestRedirect.Protocol
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
			filters = append(filters, state.HTTPFilter{
				Type: state.HTTPHeaderFilterType,
				Redirect: state.HTTPRedirectFilter{
					Scheme:   scheme,
					Hostname: hostname,
					Port:     port,
					Status:   statusCode,
				},
			})
		}
	}
	return filters
}

func httpHeadersToMap(headers []gw.HTTPHeader) map[string]string {
	resolved := make(map[string]string)
	for _, header := range headers {
		resolved[string(header.Name)] = header.Value
	}
	return resolved
}
