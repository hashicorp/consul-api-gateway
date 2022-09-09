package health

import (
	"context"
	"fmt"
	"io"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/mitchellh/cli"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["health"] = func() (cli.Command, error) {
		return NewCommand(ctx, ui, logOutput), nil
	}
}

type Command struct {
	*common.ClientCLI
}

func NewCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	return &Command{
		ClientCLI: common.NewClientCLI(ctx, help, synopsis, ui, logOutput, "health"),
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
	synopsis = "Gets the health of all registered Consul Gateway API controllers and deployments"
	help     = `
Usage: consul-api-gateway controller health [options]

  Gets Consul Gateway API controller health.

  Additional flags and more advanced use cases are detailed below.
`
)
