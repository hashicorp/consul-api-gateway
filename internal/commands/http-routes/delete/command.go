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
	if err := client.V1().DeleteHTTPRouteInNamespace(c.Context(), c.Namespace(), name); err != nil {
		return c.Error("sending the request", err)
	}
	return c.Success(fmt.Sprintf("Successfully deleted http-route: %s", name))
}

const (
	synopsis = "Deletes an HTTPRoute"
	help     = `
Usage: consul-api-gateway http-routes delete [options] NAME

  Deletes an HTTPRoute with the given NAME.

  Additional flags and more advanced use cases are detailed below.
`
)
