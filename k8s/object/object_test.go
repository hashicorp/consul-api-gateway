package object

import (
	"testing"

	"github.com/stretchr/testify/require"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestObject(t *testing.T) {
	route := &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"foo"},
		},
		Status: gw.HTTPRouteStatus{
			RouteStatus: gw.RouteStatus{
				Parents: []gw.RouteParentStatus{
					{
						Controller: "foo",
					},
				},
			},
		},
	}

	obj := New(route)
	_, ok := obj.spec.Interface().(*gw.HTTPRouteSpec)
	require.True(t, ok)

	obj.Status.Mutate(func(status interface{}) interface{} {
		httpStatus, ok := status.(*gw.HTTPRouteStatus)
		require.True(t, ok)
		return httpStatus
	})
	require.False(t, obj.Status.IsDirty())

	obj.Status.Mutate(func(status interface{}) interface{} {
		httpStatus, _ := status.(*gw.HTTPRouteStatus)
		httpStatus.Parents[0].Controller = "test"
		return status
	})
	require.True(t, obj.Status.IsDirty())
	require.Equal(t, "test", route.Status.Parents[0].Controller)
}
