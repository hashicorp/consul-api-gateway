package reconciler

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestHTTPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http-namespace/name", HTTPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}

func TestConvertHTTPRoute(t *testing.T) {
	t.Parallel()

	path := "/"
	method := gw.HTTPMethodPost
	matchType := gw.PathMatchExact
	queryMatchType := gw.QueryParamMatchExact
	headerMatchType := gw.HeaderMatchExact
	weight := int32(10)
	protocol := "https"
	hostname := gw.Hostname("example.com")
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
			}, {
				Type: service.HTTPRouteReference,
			}},
		},
		expected: `
{
	"Meta": null,
	"Name": "kitchen-sink",
	"Namespace": "",
	"Hostnames": [
		"*"
	],
	"Rules": [
		{
			"Matches": [
				{
					"Path": {
						"Type": 1,
						"Value": "/"
					},
					"Headers": [
						{
							"Type": 1,
							"Name": "a",
							"Value": "b"
						}
					],
					"Query": [
						{
							"Type": 1,
							"Name": "a",
							"Value": "b"
						}
					],
					"Method": 7
				}
			],
			"Filters": [
				{
					"Type": 1,
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
					"Type": 0,
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
	"Name": "hostnames",
	"Namespace": "",
	"Hostnames": [
		"*"
	],
	"Rules": []
}
`,
	}} {
		t.Run(test.name, func(t *testing.T) {
			resolved := convertHTTPRoute(test.namespace, test.hostname, test.name, test.meta, test.route, &K8sRoute{references: test.references})

			data, err := json.MarshalIndent(resolved, "", "  ")
			require.NoError(t, err)
			require.JSONEq(t, test.expected, string(data))
		})
	}
}
