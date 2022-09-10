package health

import (
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/testing/vm"
	"github.com/stretchr/testify/assert"
)

func TestHealth(t *testing.T) {
	controller := vm.TestController(t)
	_ = controller.PeerController(t)

	for _, tt := range []struct {
		name           string
		failure        bool
		expectedOutput string
	}{{
		name:           "success",
		expectedOutput: "Successfully retrieved controller health",
	}} {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exitStatus := 0
			if tt.failure {
				exitStatus = 1
			}

			controller.RunCLI(t, vm.CLITest{
				Command:    NewCommand,
				ExitStatus: exitStatus,
				OutputCheck: func(t *testing.T, output string) {
					assert.Contains(t, output, tt.expectedOutput)
				},
			})
		})
	}
}
