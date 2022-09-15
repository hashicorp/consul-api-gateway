package health

import (
	"context"
	"fmt"
	"io"

	commonCLI "github.com/hashicorp/consul-api-gateway/internal/cli"
	"github.com/mitchellh/cli"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["health"] = func() (cli.Command, error) {
		return NewCommand(ctx, ui, logOutput), nil
	}
}

type Command struct {
	*commonCLI.ClientCLI
}

func NewCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command {
	return &Command{
		ClientCLI: commonCLI.NewClientCLI(ctx, help, synopsis, ui, logOutput, "health"),
	}
}

func (c *Command) Run(args []string) int {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	client, err := c.CreateClient()
	if err != nil {
		return c.Error("creating the client", err)
	}

	health, err := client.V1().Health(c.Context())
	if err != nil {
		return c.Error("sending the request", err)
	}

	return c.Success(fmt.Sprintf("Successfully retrieved controller health: %v", health))
}

const (
	synopsis = "Gets the health of all registered Consul API Gateway controllers and gateways"
	help     = `
Usage: consul-api-gateway health [options]

  Gets Consul API Gateway controller and gateway health.

  Additional flags and more advanced use cases are detailed below.
`
)
