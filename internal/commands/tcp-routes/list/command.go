package list

import (
	"context"
	"fmt"
	"io"

	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/mitchellh/cli"
)

type Command struct {
	*common.ClientCLIWithNamespace

	help string

	flagAllNamespaces bool // list from all namespaces
}

func New(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	cmd := &Command{
		ClientCLIWithNamespace: common.NewClientCLIWithNamespace(ctx, help, synopsis, ui, logOutput, "get"),
	}
	cmd.Flags.BoolVar(&cmd.flagAllNamespaces, "all", false, "List gateways in all namespaces.")
	cmd.help = common.FlagUsage(help, cmd.Flags)

	return cmd
}

func (c *Command) Run(args []string) int {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	namespace := c.Namespace()
	if c.flagAllNamespaces {
		namespace = v1.AllNamespaces
	}

	client, err := c.CreateClient()
	if err != nil {
		return c.Error("creating the client", err)
	}
	routes, err := client.V1().ListTCPRoutesInNamespace(c.Context(), namespace)
	if err != nil {
		return c.Error("sending the request", err)
	}
	return c.Success(fmt.Sprintf("Successfully retrieved TCPRoutes: %v", routes))
}

func (c *Command) Help() string {
	return c.help
}

const (
	synopsis = "Lists configured TCPRoutes"
	help     = `
Usage: consul-api-gateway tcp-routes list [options]

  Lists configured TCPRoutes.

  Additional flags and more advanced use cases are detailed below.
`
)
