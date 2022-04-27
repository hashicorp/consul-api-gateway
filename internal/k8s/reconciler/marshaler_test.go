package reconciler

import (
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestMarshalRoute(t *testing.T) {
	r := &gw.HTTPRoute{}
	r.SetGroupVersionKind(schema.GroupVersionKind{
		Kind: "HTTPRoute",
	})

	route := NewRoute(r, state.NewRouteState())

	data, err := route.MarshalJSON()
	require.NoError(t, err)

	unmarshaled := &K8sRoute{}
	err = unmarshaled.UnmarshalJSON(data)
	require.NoError(t, err)

	_, ok := unmarshaled.Route.(*gw.HTTPRoute)
	require.True(t, ok)
}
