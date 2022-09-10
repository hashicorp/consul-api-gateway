package gateways

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/testing/vm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPut_Fixtures(t *testing.T) {
	controller := vm.TestController(t)

	// create a valid Vault cert to use
	_, err := controller.Vault.Client.KVv2("secret").Put(context.Background(), "certificate", map[string]interface{}{
		"server.cert": "1234",
		"server.key":  "5678",
	})
	require.NoError(t, err)

	for _, fixture := range getFixtures(t, "put") {
		fixture := fixture

		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			controller.RunCLI(t, vm.CLITest{
				Command:    NewPutCommand,
				ExitStatus: fixture.exitCode,
				Args:       []string{fixture.inputPath},
				OutputCheck: func(t *testing.T, output string) {
					if generate {
						require.NoError(t, os.WriteFile(fixture.outputPath, []byte(output), 0644))
					}
					data, err := os.ReadFile(fixture.outputPath)
					require.NoError(t, err)

					expected := string(data)
					assert.Equal(t, expected, output)
				},
			})
		})
	}
}
