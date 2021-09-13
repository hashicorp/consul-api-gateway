package consul

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
)

func portNumber(port int) *gw.PortNumber {
	p := gw.PortNumber(port)
	return &p
}

func Test_computeConfigEntriesForHTTPRoutes_simple(t *testing.T) {
	gateway := &ResolvedGateway{
		name: types.NamespacedName{Name: "prod-web", Namespace: "default"},
		listeners: map[string]*resolvedListener{
			"default": {
				name:     "default",
				protocol: "HTTP",
				port:     80,
				tls:      false,
				hostname: "",
				httpRouteBindings: []*gw.HTTPRoute{
					{
						TypeMeta: metav1.TypeMeta{
							Kind:       "HTTPRoute",
							APIVersion: gw.GroupName + "/v1alpha2",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Spec: gw.HTTPRouteSpec{
							Rules: []gw.HTTPRouteRule{
								{
									BackendRefs: []gw.HTTPBackendRef{
										{
											BackendRef: gw.BackendRef{
												BackendObjectReference: gw.BackendObjectReference{
													Name: "foo-svc",
													Port: portNumber(8080),
												},
											},
										},
									},
								},
							},
							CommonRouteSpec: gw.CommonRouteSpec{ParentRefs: []gw.ParentRef{
								{
									Name: "prod-web",
								},
							}},
						},
					},
				},
			},
		},
	}

	igwCE, routers, splitters, defaults, err := gateway.computeConfigEntries()
	require := require.New(t)
	require.NoError(err)

	igw, ok := igwCE.(*api.IngressGatewayConfigEntry)
	require.True(ok)

	require.Len(igw.Listeners, 1)
	listener := igw.Listeners[0]
	require.Equal(80, listener.Port)

	require.Len(listener.Services, 1)
	service := listener.Services[0]

	require.Equal(1, routers.Count())
	router, ok := routers.ToArray()[0].(*api.ServiceRouterConfigEntry)
	require.True(ok)
	require.Equal(service.Name, router.Name)
	require.Len(router.Routes, 1)
	require.NotNil(router.Routes[0].Destination)
	require.Equal("foo-svc", router.Routes[0].Destination.Service)

	require.Zero(splitters.Count())
	require.Equal(1, defaults.Count())
	require.Equal(service.Name, defaults.ToArray()[0].GetName())
}
