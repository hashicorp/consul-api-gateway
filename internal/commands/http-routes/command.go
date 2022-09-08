package http_routes

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

const synopsis = "Manage Consul API Gateway HTTPRoutes"
const help = `
Usage: consul-api-gateway http-routes <subcommand> [options] [args]
  This command has subcommands for interacting with Consul API Gateway
  TCPRoute configuration objects. Here are some simple examples, and more
  detailed examples are available in the subcommands or the documentation.

  Create or update the route defined in "route.json":

    $ consul-api-gateway http-routes put route.json

  Read the configuration for the http-route named "my-route" value back:

    $ consul-api-gateway http-routes get my-route

  List configured http-routes:

    $ consul-api-gateway http-routes list

  Finally, delete the http-route named "my-route":

    $ consul-api-gateway http-routes delete my-route

  For more examples, ask for subcommand help or view the documentation at
  https://www.consul.io/docs/api-gateway.
`
