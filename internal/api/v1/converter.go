package v1

import "github.com/hashicorp/consul-api-gateway/internal/core"

type HTTPRouteConverter struct {
	namespace string
	hostname  string
	prefix    string
	meta      map[string]string
	route     *HTTPRoute
}

type HTTPRouteConverterConfig struct {
	Namespace string
	Hostname  string
	Prefix    string
	Meta      map[string]string
	Route     *HTTPRoute
}

func NewHTTPRouteConverter(config HTTPRouteConverterConfig) *HTTPRouteConverter {
	return &HTTPRouteConverter{
		namespace: config.Namespace,
		hostname:  config.Hostname,
		prefix:    config.Prefix,
		meta:      config.Meta,
		route:     config.Route,
	}
}

func (c *HTTPRouteConverter) Convert() core.ResolvedRoute {
	hostnames := []string{}
	for _, hostname := range c.route.Hostnames {
		hostnames = append(hostnames, hostname)
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
		WithRules(convertRules(c.route.Rules)).
		Build()
}

var methodMappings = map[HTTPMatchMethod]core.HTTPMethod{
	HTTPMatchMethodCONNECT: core.HTTPMethodConnect,
	HTTPMatchMethodDELETE:  core.HTTPMethodDelete,
	HTTPMatchMethodPOST:    core.HTTPMethodPost,
	HTTPMatchMethodPUT:     core.HTTPMethodPut,
	HTTPMatchMethodPATCH:   core.HTTPMethodPatch,
	HTTPMatchMethodGET:     core.HTTPMethodGet,
	HTTPMatchMethodOPTIONS: core.HTTPMethodOptions,
	HTTPMatchMethodTRACE:   core.HTTPMethodTrace,
	HTTPMatchMethodHEAD:    core.HTTPMethodHead,
}

var pathMappings = map[HTTPPathMatchMatchOn]core.HTTPPathMatchType{
	HTTPPathMatchMatchOnExact:             core.HTTPPathMatchExactType,
	HTTPPathMatchMatchOnPrefix:            core.HTTPPathMatchPrefixType,
	HTTPPathMatchMatchOnRegularExpression: core.HTTPPathMatchRegularExpressionType,
}

var queryMappings = map[HTTPQueryMatchMatchOn]core.HTTPQueryMatchType{
	HTTPQueryMatchMatchOnExact:             core.HTTPQueryMatchExactType,
	HTTPQueryMatchMatchOnPresent:           core.HTTPQueryMatchPresentType,
	HTTPQueryMatchMatchOnRegularExpression: core.HTTPQueryMatchRegularExpressionType,
}

var headerMappings = map[HTTPHeaderMatchMatchOn]core.HTTPHeaderMatchType{
	HTTPHeaderMatchMatchOnExact:             core.HTTPHeaderMatchExactType,
	HTTPHeaderMatchMatchOnPrefix:            core.HTTPHeaderMatchPrefixType,
	HTTPHeaderMatchMatchOnPresent:           core.HTTPHeaderMatchPresentType,
	HTTPHeaderMatchMatchOnRegularExpression: core.HTTPHeaderMatchRegularExpressionType,
	HTTPHeaderMatchMatchOnSuffix:            core.HTTPHeaderMatchSuffixType,
}

func convertRules(rules []HTTPRouteRule) []core.HTTPRouteRule {
	resolved := []core.HTTPRouteRule{}
	for _, r := range rules {
		filters := convertHTTPRouteFilters(r.Filters)
		matches := []core.HTTPMatch{}
		for _, match := range r.Matches {
			stateMatch := core.HTTPMatch{}
			if match.Method != nil {
				if method, found := methodMappings[*match.Method]; found {
					stateMatch.Method = method
				}
			}
			if match.Path != nil {
				matchType := HTTPPathMatchMatchOnExact
				if match.Path.MatchOn != "" {
					matchType = match.Path.MatchOn
				}
				if mappedType, found := pathMappings[matchType]; found {
					stateMatch.Path = core.HTTPPathMatch{
						Type:  mappedType,
						Value: match.Path.Value,
					}
				}
			} else {
				stateMatch.Path = core.HTTPPathMatch{
					Type:  core.HTTPPathMatchPrefixType,
					Value: "/",
				}
			}
			for _, param := range match.Query {
				matchType := HTTPQueryMatchMatchOnExact
				if param.MatchOn != "" {
					matchType = param.MatchOn
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
				matchType := HTTPHeaderMatchMatchOnExact
				if header.MatchOn != "" {
					matchType = header.MatchOn
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
		for _, service := range r.Services {
			weight := int32(1)
			if service.Weight != nil {
				weight = int32(*service.Weight)
			}
			// note that we ignore the Port value
			services = append(services, core.HTTPService{
				Service: core.ResolvedService{
					ConsulNamespace: stringOrEmpty(service.Namespace),
					Service:         service.Name,
				},
				Weight:  weight,
				Filters: convertHTTPRouteFilters(service.Filters),
			})
		}
		resolved = append(resolved, core.HTTPRouteRule{
			Filters:  filters,
			Matches:  matches,
			Services: services,
		})
	}
	return resolved
}

func convertHTTPRouteFilters(routeFilters *HTTPFilters) []core.HTTPFilter {
	if routeFilters == nil {
		return nil
	}

	filters := []core.HTTPFilter{}
	for _, filter := range routeFilters.Headers {
		filters = append(filters, core.HTTPFilter{
			Type: core.HTTPHeaderFilterType,
			Header: core.HTTPHeaderFilter{
				Set:    mapOrInit(filter.Set),
				Add:    mapOrInit(filter.Add),
				Remove: filter.Remove,
			},
		})
	}
	return filters
}

func mapOrInit(m *map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return *m
}

// func httpHeadersToMap(headers []gwv1alpha2.HTTPHeader) map[string]string {
// 	resolved := make(map[string]string)
// 	for _, header := range headers {
// 		resolved[string(header.Name)] = header.Value
// 	}
// 	return resolved
// }
