package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGatewayID(t *testing.T) {
	t.Parallel()

	gw := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}

	gwState := state.InitialGatewayState(gw)
	gwState.ConsulNamespace = "consul"

	gateway := newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, gwState)
	require.Equal(t, internalCore.GatewayID{Service: "name", ConsulNamespace: "consul"}, gateway.ID())
}

func TestK8sGateway_Resolve(t *testing.T) {
	t.Parallel()

	gcc := apigwv1alpha1.GatewayClassConfig{}

	gw := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{Name: "name", Namespace: "namespace"},
	}

	gwState := state.InitialGatewayState(gw)
	gwState.ConsulNamespace = "consul"

	// Verify max connections not set if unset on GatewayClassConfig
	resolvedGateway := newK8sGateway(gcc, gw, gwState).Resolve()
	assert.Nil(t, resolvedGateway.MaxConnections)
	assert.Equal(t, "consul-api-gateway", resolvedGateway.Meta[gatewayMetaExternalSource])
	assert.Equal(t, "name", resolvedGateway.Meta[gatewayMetaName])
	assert.Equal(t, "namespace", resolvedGateway.Meta[gatewayMetaNamespace])

	// Verify max connections set if set on GatewayClassConfig
	gcc.Spec.ConnectionManagement.MaxConnections = pointer.Int32(100)
	resolvedGateway = newK8sGateway(gcc, gw, gwState).Resolve()
	require.NotNil(t, resolvedGateway.MaxConnections)
	assert.EqualValues(t, 100, *resolvedGateway.MaxConnections)
}
