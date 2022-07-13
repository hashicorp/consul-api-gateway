package reconciler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

func TestHTTPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http-namespace/name", HTTPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}

func TestConvertHTTPRoute(t *testing.T) {
	t.Parallel()

	path := "/"
	method := gwv1alpha2.HTTPMethodPost
	matchType := gwv1alpha2.PathMatchExact
	queryMatchType := gwv1alpha2.QueryParamMatchExact
	headerMatchType := gwv1alpha2.HeaderMatchExact
	weight := int32(10)
	protocol := "https"
	hostname := gwv1alpha2.PreciseHostname("example.com")
	port := gwv1alpha2.PortNumber(8443)
	statusCode := 302
	for _, test := range []struct {
		name       string
		namespace  string
		hostname   string
		meta       map[string]string
		route      *gwv1alpha2.HTTPRoute
		references service.RouteRuleReferenceMap
		expected   string
	}{{
		name:  "kitchen-sink",
		route: &gwv1alpha2.HTTPRoute{},
		references: service.RouteRuleReferenceMap{
			service.RouteRule{
				HTTPRule: &gwv1alpha2.HTTPRouteRule{
					Matches: []gwv1alpha2.HTTPRouteMatch{{
						Method: &method,
						Path: &gwv1alpha2.HTTPPathMatch{
							Value: &path,
							Type:  &matchType,
						},
						QueryParams: []gwv1alpha2.HTTPQueryParamMatch{{
							Type:  &queryMatchType,
							Name:  "a",
							Value: "b",
						}},
						Headers: []gwv1alpha2.HTTPHeaderMatch{{
							Type:  &headerMatchType,
							Name:  gwv1alpha2.HTTPHeaderName("a"),
							Value: "b",
						}},
					}},
					Filters: []gwv1alpha2.HTTPRouteFilter{{
						Type: gwv1alpha2.HTTPRouteFilterRequestRedirect,
						RequestRedirect: &gwv1alpha2.HTTPRequestRedirectFilter{
							Scheme:     &protocol,
							Hostname:   &hostname,
							Port:       &port,
							StatusCode: &statusCode,
						},
					}, {
						Type: gwv1alpha2.HTTPRouteFilterRequestHeaderModifier,
						RequestHeaderModifier: &gwv1alpha2.HTTPRequestHeaderFilter{
							Set: []gwv1alpha2.HTTPHeader{{
								Name:  "x-a",
								Value: "a",
							}},
							Add: []gwv1alpha2.HTTPHeader{{
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
					HTTPRef: &gwv1alpha2.HTTPBackendRef{
						BackendRef: gwv1alpha2.BackendRef{
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
					},
					"URLRewrite": {
						"Type": 0,
						"ReplacePrefixMatch": ""
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
					},
					"URLRewrite": {
						"Type": 0,
						"ReplacePrefixMatch": ""
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
		route: &gwv1alpha2.HTTPRoute{
			Spec: gwv1alpha2.HTTPRouteSpec{
				Hostnames: []gwv1alpha2.Hostname{"*"},
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
