package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func HTTPRouteID(namespacedName types.NamespacedName) string {
	return "http-" + namespacedName.String()
}

func convertHTTPRoute(namespace, hostname, prefix string, meta map[string]string, route *gw.HTTPRoute, k8sRoute *K8sRoute) *core.ResolvedRoute {
	hostnames := []string{}
	for _, hostname := range route.Spec.Hostnames {
		hostnames = append(hostnames, string(hostname))
	}
	if len(hostnames) == 0 {
		if hostname == "" {
			hostname = "*"
		}
		hostnames = append(hostnames, hostname)
	}
	name := prefix + route.Name

	resolved := core.NewHTTPRouteBuilder().
		WithName(name).
		// we always use the listener namespace for the routes
		// themselves, while the services they route to might
		// be in different namespaces
		WithNamespace(namespace).
		WithHostnames(hostnames).
		WithMeta(meta).
		WithRules(httpReferencesToRules(k8sRoute.references)).
		Build()
	return &resolved
}

var methodMappings = map[gw.HTTPMethod]core.HTTPMethod{
	gw.HTTPMethodConnect: core.HTTPMethodConnect,
	gw.HTTPMethodDelete:  core.HTTPMethodDelete,
	gw.HTTPMethodPost:    core.HTTPMethodPost,
	gw.HTTPMethodPut:     core.HTTPMethodPut,
	gw.HTTPMethodPatch:   core.HTTPMethodPatch,
	gw.HTTPMethodGet:     core.HTTPMethodGet,
	gw.HTTPMethodOptions: core.HTTPMethodOptions,
	gw.HTTPMethodTrace:   core.HTTPMethodTrace,
	gw.HTTPMethodHead:    core.HTTPMethodHead,
}

var pathMappings = map[gw.PathMatchType]core.HTTPPathMatchType{
	gw.PathMatchExact:             core.HTTPPathMatchExactType,
	gw.PathMatchRegularExpression: core.HTTPPathMatchRegularExpressionType,
}

var queryMappings = map[gw.QueryParamMatchType]core.HTTPQueryMatchType{
	gw.QueryParamMatchExact: core.HTTPQueryMatchExactType,
}

var headerMappings = map[gw.HeaderMatchType]core.HTTPHeaderMatchType{
	gw.HeaderMatchExact:             core.HTTPHeaderMatchExactType,
	gw.HeaderMatchRegularExpression: core.HTTPHeaderMatchRegularExpressionType,
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
				matchType := gw.PathMatchExact
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
				matchType := gw.QueryParamMatchExact
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
				matchType := gw.HeaderMatchExact
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
				// TODO: support other reference types
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

func convertHTTPRouteFilters(routeFilters []gw.HTTPRouteFilter) []core.HTTPFilter {
	filters := []core.HTTPFilter{}
	for _, filter := range routeFilters {
		switch filter.Type {
		case gw.HTTPRouteFilterRequestHeaderModifier:
			filters = append(filters, core.HTTPFilter{
				Type: core.HTTPHeaderFilterType,
				Header: core.HTTPHeaderFilter{
					Set:    httpHeadersToMap(filter.RequestHeaderModifier.Set),
					Add:    httpHeadersToMap(filter.RequestHeaderModifier.Add),
					Remove: filter.RequestHeaderModifier.Remove,
				},
			})
		case gw.HTTPRouteFilterRequestRedirect:
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
