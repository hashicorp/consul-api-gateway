package controller

import (
	"context"
	"io"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/mitchellh/cli"
)

type HealthCommand struct {
	*common.ClientCLI
}

func NewHealthCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) *HealthCommand {
	return &HealthCommand{
		ClientCLI: common.NewClientCLI(ctx, healthHelp, healthSynopsis, ui, logOutput, "health"),
	}
}

func (c *HealthCommand) Run(args []string) int {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	// client, err := c.CreateClient()
	// if err != nil {
	// 	return c.Error("creating the client", err)
	// }
	//
	// health, err := client.Internal().GetControllerHealth(c.Context(), c.Namespace(), name)
	// if err != nil {
	// 	return c.Error("sending the request", err)
	// }
	//
	// return c.Success(fmt.Sprintf("Successfully retrieved controller health: %v", health))

	return c.Success("health")
}

const (
	healthSynopsis = "Gets the health of all registered Consul API Gateway controllers"
	healthHelp     = `
Usage: consul-api-gateway controller health [options]

  Gets Consul API Gateway controller health.

  Additional flags and more advanced use cases are detailed below.
`
)
