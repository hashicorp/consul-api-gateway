package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGatewayID(t *testing.T) {
	t.Parallel()

	gw := &gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}
	gateway := NewGateway(v1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState("consul", gw))

	require.Equal(t, internalCore.GatewayID{Service: "name", ConsulNamespace: "consul"}, gateway.ID())
}
