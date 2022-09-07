package delete

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/mitchellh/cli"
)

type Command struct {
	*common.ClientCLIWithNamespace
}

func New(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	return &Command{
		ClientCLIWithNamespace: common.NewClientCLIWithNamespace(ctx, help, synopsis, ui, logOutput, "delete"),
	}
}

func (c *Command) Run(args []string) int {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	name := c.Flags.Arg(0)
	if name == "" {
		return c.Error("parsing arguments", errors.New("a name parameter must be provided"))
	}
	client, err := c.CreateClient()
	if err != nil {
		return c.Error("creating the client", err)
	}
	if err := client.V1().DeleteGatewayInNamespace(c.Context(), c.Namespace(), name); err != nil {
		return c.Error("sending the request", err)
	}
	return c.Success(fmt.Sprintf("Successfully deleted Gateway: %s", name))
}

const (
	synopsis = "Deletes a Gateway"
	help     = `
Usage: consul-api-gateway gateways delete [options] NAME

  Deletes a Gateway with the given NAME.

  Additional flags and more advanced use cases are detailed below.
`
)
