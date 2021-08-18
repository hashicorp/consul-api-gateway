package consul

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"github.com/hashicorp/consul/api"
)

func portNumber(port int) *gw.PortNumber {
	p := gw.PortNumber(port)
	return &p
}

func Test_computeConfigEntriesForHTTPRoutes_simple(t *testing.T) {
	gateway := types.NamespacedName{Name: "prod-web", Namespace: "default"}
	listener := &gw.Listener{
		Port:     80,
		Protocol: "HTTP",
		Routes: gw.RouteBindingSelector{
			Kind: "HTTPRoute",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"gateway": "prod-web-gw"},
			},
		},
	}
	routes := []*gw.HTTPRoute{
		{
			TypeMeta: HTTPRouteType,
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels:    map[string]string{"gateway": "prod-web-gw"},
			},
			Spec: gw.HTTPRouteSpec{
				Rules: []gw.HTTPRouteRule{
					{
						ForwardTo: []gw.HTTPRouteForwardTo{
							{
								ServiceName: pointer.String("foo-svc"),
								Port:        portNumber(8080),
							},
						},
					},
				},
			},
		},
	}

	services, routers, splitters, defaults := computeConfigEntriesForHTTPRoutes(gateway, listener, routes)
	require := require.New(t)
	require.Len(services, 1)
	service := services[0]
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
