package tcproutes

import (
	"context"
	"errors"
	"fmt"
	"io"

	commonCLI "github.com/hashicorp/consul-api-gateway/internal/cli"
	"github.com/mitchellh/cli"
)

type DeleteCommand struct {
	*commonCLI.ClientCLIWithNamespace
}

func NewDeleteCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command {
	return &DeleteCommand{
		ClientCLIWithNamespace: commonCLI.NewClientCLIWithNamespace(ctx, deleteHelp, deleteSynopsis, ui, logOutput, "delete"),
	}
}

func (c *DeleteCommand) Run(args []string) int {
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

	if err := client.V1().DeleteTCPRouteInNamespace(c.Context(), c.Namespace(), name); err != nil {
		return c.Error("sending the request", err)
	}

	return c.Success(fmt.Sprintf("Successfully deleted tcp-route: %s", name))
}

const (
	deleteSynopsis = "Deletes a TCPRoute"
	deleteHelp     = `
Usage: consul-api-gateway tcp-routes delete [options] NAME

  Deletes a TCPRoute with the given NAME.

  Additional flags and more advanced use cases are detailed below.
`
)
