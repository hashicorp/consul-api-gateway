package tcproutes

import (
	"context"
	"fmt"
	"io"

	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	commonCLI "github.com/hashicorp/consul-api-gateway/internal/cli"
	"github.com/mitchellh/cli"
)

type ListCommand struct {
	*commonCLI.ClientCLIWithNamespace

	help string

	flagAllNamespaces bool // list from all namespaces
}

func NewListCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command {
	cmd := &ListCommand{
		ClientCLIWithNamespace: commonCLI.NewClientCLIWithNamespace(ctx, listHelp, listSynopsis, ui, logOutput, "get"),
	}
	cmd.Flags.BoolVar(&cmd.flagAllNamespaces, "all-namespaces", false, "List tcp-routes in all namespaces.")
	cmd.help = commonCLI.FlagUsage(help, cmd.Flags)

	return cmd
}

func (c *ListCommand) Run(args []string) int {
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

func (c *ListCommand) Help() string {
	return c.help
}

const (
	listSynopsis = "Lists TCPRoutes"
	listHelp     = `
Usage: consul-api-gateway tcp-routes list [options]

  Lists TCPRoutes.

  Additional flags and more advanced use cases are detailed below.
`
)
