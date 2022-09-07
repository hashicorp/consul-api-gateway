package gateways

import (
	"github.com/mitchellh/cli"
)

func New() *Command {
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
  This command has subcommands for interacting with Consul API Gateways
  configuration objects. Here are some simple examples, and more detailed examples are
  available in the subcommands or the documentation.
  Create or update the gateway defined in "gateway.json":
    $ consul-api-gateway gateways put gateway.json
  Read the configuration for the gateway named "my-gateway" value back:
    $ consul-api-gateway gateways get my-gateway
  List configured gateways:
    $ consul-api-gateway gateways list
  Finally, delete the gateway named "my-gateway":
    $ consul-api-gateway gateways delete my-gateway
  For more examples, ask for subcommand help or view the documentation.
`
