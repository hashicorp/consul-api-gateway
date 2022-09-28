package health

import (
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/testing/vm"
	"github.com/stretchr/testify/assert"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	controller := vm.TestController(t)

	expectedOutput := "Successfully retrieved controller health"

	controller.RunCLI(t, vm.CLITest{
		Command: NewCommand,
		OutputCheck: func(t *testing.T, output string) {
			assert.Contains(t, output, expectedOutput)
		},
	})
}

func TestHealth_MultiControllers(t *testing.T) {
	t.Parallel()

	controller := vm.TestController(t)
	_ = controller.PeerController(t)

	expectedOutput := "Successfully retrieved controller health"

	controller.RunCLI(t, vm.CLITest{
		Command: NewCommand,
		OutputCheck: func(t *testing.T, output string) {
			assert.Contains(t, output, expectedOutput)
		},
	})
}
