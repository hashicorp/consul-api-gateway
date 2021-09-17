package routes

import (
	"testing"

	"github.com/stretchr/testify/require"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func Test_getRouteStatusPtr(t *testing.T) {
	route := &gw.HTTPRoute{
		Status: gw.HTTPRouteStatus{RouteStatus: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{
				{
					Controller: "test",
				},
			},
		}},
	}

	status := getRouteStatusPtr(route)
	require.NotNil(t, status)
	require.Len(t, status.Parents, 1)
	require.Equal(t, gw.GatewayController("test"), status.Parents[0].Controller)

	status.Parents[0].Controller = "foo"
	require.Equal(t, gw.GatewayController("foo"), route.Status.RouteStatus.Parents[0].Controller)
}
