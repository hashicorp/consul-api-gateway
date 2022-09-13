package reconciler

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestMarshalRoute(t *testing.T) {
	r := &gwv1alpha2.HTTPRoute{}
	r.SetGroupVersionKind(schema.GroupVersionKind{
		Kind: "HTTPRoute",
	})

	route := newK8sRoute(r, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})

	data, err := route.MarshalJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	unmarshaled := &K8sRoute{}
	require.NoError(t, unmarshaled.UnmarshalJSON(data))

	_, ok := unmarshaled.Route.(*gwv1alpha2.HTTPRoute)
	assert.True(t, ok)
}
