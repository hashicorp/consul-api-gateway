package converters

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

func TestConvertHTTPRoute(t *testing.T) {
	t.Parallel()

	path := "/"
	method := gw.HTTPMethodPost
	matchType := gw.PathMatchExact
	queryMatchType := gw.QueryParamMatchExact
	headerMatchType := gw.HeaderMatchExact
	weight := int32(10)
	protocol := "https"
	hostname := gw.PreciseHostname("example.com")
	port := gw.PortNumber(8443)
	statusCode := 302
	for _, test := range []struct {
		name       string
		namespace  string
		hostname   string
		meta       map[string]string
		route      *gw.HTTPRoute
		references service.RouteRuleReferenceMap
		expected   string
	}{{
		name:  "kitchen-sink",
		route: &gw.HTTPRoute{},
		references: service.RouteRuleReferenceMap{
			service.RouteRule{
				HTTPRule: &gw.HTTPRouteRule{
					Matches: []gw.HTTPRouteMatch{{
						Method: &method,
						Path: &gw.HTTPPathMatch{
							Value: &path,
							Type:  &matchType,
						},
						QueryParams: []gw.HTTPQueryParamMatch{{
							Type:  &queryMatchType,
							Name:  "a",
							Value: "b",
						}},
						Headers: []gw.HTTPHeaderMatch{{
							Type:  &headerMatchType,
							Name:  gw.HTTPHeaderName("a"),
							Value: "b",
						}},
					}},
					Filters: []gw.HTTPRouteFilter{{
						Type: gw.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gw.HTTPRequestRedirectFilter{
							Scheme:     &protocol,
							Hostname:   &hostname,
							Port:       &port,
							StatusCode: &statusCode,
						},
					}, {
						Type: gw.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gw.HTTPRequestHeaderFilter{
							Set: []gw.HTTPHeader{{
								Name:  "x-a",
								Value: "a",
							}},
							Add: []gw.HTTPHeader{{
								Name:  "x-b",
								Value: "b",
							}},
							Remove: []string{"x-c"},
						},
					}},
				},
			}: []service.ResolvedReference{{
				Type: service.ConsulServiceReference,
				Consul: &service.ConsulService{
					Name:      "name",
					Namespace: "namespace",
				},
				Reference: &service.BackendReference{
					HTTPRef: &gw.HTTPBackendRef{
						BackendRef: gw.BackendRef{
							Weight: &weight,
						},
					},
				},
			}},
		},
		expected: `
{
	"Meta": null,
	"Name": "",
	"Namespace": "",
	"Hostnames": [
		"*"
	],
	"Rules": [
		{
			"Matches": [
				{
					"Path": {
						"Type": "HTTPPathMatchExact",
						"Value": "/"
					},
					"Headers": [
						{
							"Type": "HTTPHeaderMatchExact",
							"Name": "a",
							"Value": "b"
						}
					],
					"Query": [
						{
							"Type": "HTTPQueryMatchExact",
							"Name": "a",
							"Value": "b"
						}
					],
					"Method": "POST"
				}
			],
			"Filters": [
				{
					"Type": "HTTPRedirectFilter",
					"Header": {
						"Set": null,
						"Add": null,
						"Remove": null
					},
					"Redirect": {
						"Scheme": "https",
						"Hostname": "example.com",
						"Port": 8443,
						"Status": 302
					}
				},
				{
					"Type": "HTTPHeaderFilter",
					"Header": {
						"Set": {
							"x-a": "a"
						},
						"Add": {
							"x-b": "b"
						},
						"Remove": [
							"x-c"
						]
					},
					"Redirect": {
						"Scheme": "",
						"Hostname": "",
						"Port": 0,
						"Status": 0
					}
				}
			],
			"Services": [
				{
					"Service": {
						"ConsulNamespace": "namespace",
						"Service": "name"
					},
					"Weight": 10,
					"Filters": []
				}
			]
		}
	]
}
`,
	}, {
		name: "hostnames",
		route: &gw.HTTPRoute{
			Spec: gw.HTTPRouteSpec{
				Hostnames: []gw.Hostname{"*"},
			},
		},
		references: service.RouteRuleReferenceMap{},
		expected: `
{
	"Meta": null,
	"Name": "",
	"Namespace": "",
	"Hostnames": [
		"*"
	],
	"Rules": []
}
`,
	}} {
		t.Run(test.name, func(t *testing.T) {
			converter := &HTTPRouteConverter{
				namespace: test.namespace,
				hostname:  test.hostname,
				meta:      test.meta,
				route:     test.route,
				state: &state.RouteState{
					References: test.references,
				},
			}
			resolved := converter.Convert()
			data, err := json.MarshalIndent(resolved, "", "  ")
			require.NoError(t, err)
			require.JSONEq(t, test.expected, string(data))
		})
	}
}
