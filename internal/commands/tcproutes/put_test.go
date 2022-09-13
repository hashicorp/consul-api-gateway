package tcproutes

import (
	"context"
	"testing"

	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	apiTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
	"github.com/hashicorp/consul-api-gateway/internal/testing/vm"
	"github.com/stretchr/testify/require"
)

func TestPut_Fixtures(t *testing.T) {
	vm.TestFixtures(t, vm.FixturesConfig{
		Command: NewPutCommand,
		Args: func(fixture vm.Fixture) []string {
			return []string{fixture.InputPath}
		},
		Setup: func(controller *vm.Controller) {
			// create some Consul service targets
			_ = controller.RegisterTCPServiceTargetWithName(t, "tcp-service-1")
			_ = controller.RegisterTCPServiceTargetWithName(t, "tcp-service-2")

			// create a couple of TCP-based gateways and HTTP-based gateways
			_, err := controller.Client.V1().CreateGateway(context.Background(), v1.Gateway{
				Listeners: []v1.Listener{{
					Name:     apiTesting.StringPtr("listener-1"),
					Port:     9091,
					Protocol: v1.ListenerProtocolHttp,
				}, {
					Name:     apiTesting.StringPtr("listener-2"),
					Port:     9092,
					Protocol: v1.ListenerProtocolHttp,
				}},
				Name: "http-1",
			})
			require.NoError(t, err)

			_, err = controller.Client.V1().CreateGateway(context.Background(), v1.Gateway{
				Listeners: []v1.Listener{{
					Name:     apiTesting.StringPtr("listener-1"),
					Port:     9095,
					Protocol: v1.ListenerProtocolTcp,
				}, {
					Name:     apiTesting.StringPtr("listener-2"),
					Port:     9096,
					Protocol: v1.ListenerProtocolTcp,
				}},
				Name: "tcp-1",
			})
			require.NoError(t, err)

			_, err = controller.Client.V1().CreateGateway(context.Background(), v1.Gateway{
				Listeners: []v1.Listener{{
					Port:     9097,
					Protocol: v1.ListenerProtocolTcp,
				}, {
					Name:     apiTesting.StringPtr("listener-2"),
					Port:     9098,
					Protocol: v1.ListenerProtocolTcp,
				}},
				Name: "tcp-2",
			})
			require.NoError(t, err)
		},
	})
}
