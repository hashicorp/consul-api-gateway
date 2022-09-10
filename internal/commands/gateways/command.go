package gateways

import (
	"context"
	"io"

	"github.com/mitchellh/cli"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["gateways"] = func() (cli.Command, error) {
		return NewCommand(), nil
	}

	commands["gateways delete"] = func() (cli.Command, error) {
		return NewDeleteCommand(ctx, ui, logOutput), nil
	}

	commands["gateways get"] = func() (cli.Command, error) {
		return NewGetCommand(ctx, ui, logOutput), nil
	}

	commands["gateways list"] = func() (cli.Command, error) {
		return NewListCommand(ctx, ui, logOutput), nil
	}

	commands["gateways put"] = func() (cli.Command, error) {
		return NewPutCommand(ctx, ui, logOutput), nil
	}
}

func NewCommand() cli.Command {
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

const synopsis = "Manage Consul API Gateways"
const help = `
Usage: consul-api-gateway gateways <subcommand> [options] [args]
  This command has subcommands for interacting with Consul API Gateway
  configuration objects. Here are some simple examples, and more detailed
  examples are available in the subcommands or the documentation.

  Create or update the gateway defined in "gateway.json":

    $ consul-api-gateway gateways put gateway.json

  Read the configuration for the gateway named "my-gateway" value back:

    $ consul-api-gateway gateways get my-gateway

  List configured gateways:

    $ consul-api-gateway gateways list

  Finally, delete the gateway named "my-gateway":

    $ consul-api-gateway gateways delete my-gateway

  For more examples, ask for subcommand help or view the documentation at
  https://www.consul.io/docs/api-gateway.
`
