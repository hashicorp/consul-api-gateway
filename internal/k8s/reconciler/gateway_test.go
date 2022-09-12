package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestGatewayID(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gw := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}

	gwState := state.InitialGatewayState(gw)
	gwState.ConsulNamespace = "consul"

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           gwState,
		ConsulNamespace: "consul",
	})
	require.Equal(t, internalCore.GatewayID{Service: "name", ConsulNamespace: "consul"}, gateway.ID())
}
