package gateways

import (
	"context"
	"testing"

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
			// create a valid Vault cert to use
			_, err := controller.Vault.Client.KVv2("secret").Put(context.Background(), "certificate", map[string]interface{}{
				"server.cert": "1234",
				"server.key":  "5678",
			})
			require.NoError(t, err)
		},
	})
}
