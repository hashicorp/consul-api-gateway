package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestMarshalRoute(t *testing.T) {
	r := &gwv1alpha2.HTTPRoute{}
	r.SetGroupVersionKind(schema.GroupVersionKind{
		Kind: "HTTPRoute",
	})

	route := newK8sRoute(r, state.NewRouteState())

	data, err := route.MarshalJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	unmarshaled := &K8sRoute{}
	require.NoError(t, unmarshaled.UnmarshalJSON(data))

	_, ok := unmarshaled.Route.(*gwv1alpha2.HTTPRoute)
	assert.True(t, ok)
}
