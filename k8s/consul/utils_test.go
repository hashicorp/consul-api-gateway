package consul

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var (
	gwAll    = gw.GatewayAllowAll
	routeAll = gw.RouteSelectAll
)

func TestRouteMatches(t *testing.T) {

	cases := []struct {
		name     string
		route    gw.HTTPRoute
		gateway  gw.Gateway
		selector gw.RouteBindingSelector
		expected bool
	}{
		{
			name: "defaults",
			route: gw.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			},
			gateway:  gw.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "default"}},
			selector: gw.RouteBindingSelector{},
			expected: true,
		},
		{
			name: "defaults different namespaces",
			route: gw.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			},
			gateway:  gw.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "test"}},
			selector: gw.RouteBindingSelector{},
			// by default only routes in the same namespace as the gateway should match
			expected: false,
		},
		{
			name: "all route all gateway match",
			route: gw.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       gw.HTTPRouteSpec{Gateways: &gw.RouteGateways{Allow: &gwAll}},
			},
			gateway: gw.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "test"}},
			selector: gw.RouteBindingSelector{
				Namespaces: &gw.RouteNamespaces{From: &routeAll},
			},
			expected: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			match := routeMatches(&tt.gateway, tt.selector, &tt.route)
			require.Equal(t, tt.expected, match)
		})
	}
}
