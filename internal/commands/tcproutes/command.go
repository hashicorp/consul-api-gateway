package tcproutes

import (
	"context"
	"io"

	"github.com/mitchellh/cli"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["tcp-routes"] = func() (cli.Command, error) {
		return NewCommand(), nil
	}

	commands["tcp-routes delete"] = func() (cli.Command, error) {
		return NewDeleteCommand(ctx, ui, logOutput), nil
	}

	commands["tcp-routes get"] = func() (cli.Command, error) {
		return NewGetCommand(ctx, ui, logOutput), nil
	}

	commands["tcp-routes list"] = func() (cli.Command, error) {
		return NewListCommand(ctx, ui, logOutput), nil
	}

	commands["tcp-routes put"] = func() (cli.Command, error) {
		return NewPutCommand(ctx, ui, logOutput), nil
	}
}

func NewCommand() *Command {
	return &Command{}
}

type Command struct{}

func (c *Command) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *Command) Synopsis() string {
	return synopsis
}

func (c *Command) Help() string {
	return help
}

const synopsis = "Manage Consul API Gateway TCPRoutes"
const help = `
Usage: consul-api-gateway http-routes <subcommand> [options] [args]
  This command has subcommands for interacting with Consul API Gateway
  TCPRoute configuration objects. Here are some simple examples, and more
  detailed examples are available in the subcommands or the documentation.

  Create or update the route defined in "route.json":

    $ consul-api-gateway tcp-routes put route.json

  Read the configuration for the tcp-route named "my-route" value back:

    $ consul-api-gateway tcp-routes get my-route

  List configured http-routes:

    $ consul-api-gateway tcp-routes list

  Finally, delete the tcp-route named "my-route":

    $ consul-api-gateway tcp-routes delete my-route

  For more examples, ask for subcommand help or view the documentation at
  https://www.consul.io/docs/api-gateway.
`