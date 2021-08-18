package consul

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var (
	gwAll    = gw.GatewayAllowAll
	routeAll = gw.RouteSelectAll
)
var (
	HTTPRouteType = metav1.TypeMeta{Kind: "HTTPRoute", APIVersion: gw.GroupVersion.String()}
)

func TestRouteMatches(t *testing.T) {

	cases := []struct {
		name     string
		route    gw.HTTPRoute
		gateway  types.NamespacedName
		selector gw.RouteBindingSelector
		expected bool
	}{
		{
			name: "defaults",
			route: gw.HTTPRoute{
				TypeMeta:   HTTPRouteType,
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			},
			gateway: types.NamespacedName{Name: "gateway", Namespace: "default"},
			selector: gw.RouteBindingSelector{
				Kind: "HTTPRoute",
			},
			expected: true,
		},
		{
			name: "defaults different namespaces",
			route: gw.HTTPRoute{
				TypeMeta:   HTTPRouteType,
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			},
			gateway: types.NamespacedName{Name: "gateway", Namespace: "test"},
			selector: gw.RouteBindingSelector{
				Kind: "HTTPRoute",
			},
			// by default only routes in the same namespace as the gateway should match
			expected: false,
		},
		{
			name: "all route all gateway match",
			route: gw.HTTPRoute{
				TypeMeta:   HTTPRouteType,
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       gw.HTTPRouteSpec{Gateways: &gw.RouteGateways{Allow: &gwAll}},
			},
			gateway: types.NamespacedName{Name: "gateway", Namespace: "test"},
			selector: gw.RouteBindingSelector{
				Kind:       "HTTPRoute",
				Namespaces: &gw.RouteNamespaces{From: &routeAll},
			},
			expected: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			match, reason := routeMatches(tt.gateway, tt.selector, &tt.route)
			require.Equal(t, tt.expected, match, reason)
		})
	}
}
