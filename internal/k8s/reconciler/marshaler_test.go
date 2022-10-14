package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestMarshalRoute(t *testing.T) {
	r := &gwv1beta1.HTTPRoute{
		Spec: gwv1beta1.HTTPRouteSpec{
			Rules: []gwv1beta1.HTTPRouteRule{{
				Filters: []gwv1beta1.HTTPRouteFilter{
					{
						Type: gwv1beta1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gwv1beta1.HTTPURLRewriteFilter{
							Path: &gwv1beta1.HTTPPathModifier{
								Type:               gwv1beta1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: pointer.String("/api/v1"),
							},
						},
					},
				}},
			},
		},
	}
	r.SetGroupVersionKind(schema.GroupVersionKind{
		Kind: "HTTPRoute",
	})

	route := newK8sRoute(r, state.NewRouteState())

	data, err := NewMarshaler().MarshalRoute(route)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	unmarshaled, err := NewMarshaler().UnmarshalRoute(data)
	require.NoError(t, err)
	require.NotNil(t, unmarshaled)

	route, ok := unmarshaled.(*K8sRoute)
	require.True(t, ok)
}

func TestMarshalGateway(t *testing.T) {
	g := &gwv1beta1.Gateway{}

	gcc := v1alpha1.GatewayClassConfig{
		Spec: v1alpha1.GatewayClassConfigSpec{
			ConnectionManagement: v1alpha1.ConnectionManagementSpec{
				MaxConnections: pointer.Int32(4096)}},
	}

	gateway := newK8sGateway(gcc, g, state.InitialGatewayState(g))

	data, err := NewMarshaler().MarshalGateway(gateway)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	unmarshaled, err := NewMarshaler().UnmarshalGateway(data)
	require.NoError(t, err)
	require.NotNil(t, unmarshaled)

	gateway, ok := unmarshaled.(*K8sGateway)
	require.True(t, ok)
	require.NotNil(t, gateway)
	require.NotNil(t, gateway.Config.Spec.ConnectionManagement.MaxConnections)
	assert.EqualValues(t, 4096, *gateway.Config.Spec.ConnectionManagement.MaxConnections)
}
