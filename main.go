package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/mitchellh/cli"

	cmdExec "github.com/hashicorp/consul-api-gateway/internal/commands/exec"
	cmdGateways "github.com/hashicorp/consul-api-gateway/internal/commands/gateways"
	cmdGatewaysDelete "github.com/hashicorp/consul-api-gateway/internal/commands/gateways/delete"
	cmdGatewaysGet "github.com/hashicorp/consul-api-gateway/internal/commands/gateways/get"
	cmdGatewaysList "github.com/hashicorp/consul-api-gateway/internal/commands/gateways/list"
	cmdGatewaysPut "github.com/hashicorp/consul-api-gateway/internal/commands/gateways/put"
	cmdHTTPRoutes "github.com/hashicorp/consul-api-gateway/internal/commands/http-routes"
	cmdHTTPRoutesDelete "github.com/hashicorp/consul-api-gateway/internal/commands/http-routes/delete"
	cmdHTTPRoutesGet "github.com/hashicorp/consul-api-gateway/internal/commands/http-routes/get"
	cmdHTTPRoutesList "github.com/hashicorp/consul-api-gateway/internal/commands/http-routes/list"
	cmdHTTPRoutesPut "github.com/hashicorp/consul-api-gateway/internal/commands/http-routes/put"
	cmdServer "github.com/hashicorp/consul-api-gateway/internal/commands/server"
	cmdTCPRoutes "github.com/hashicorp/consul-api-gateway/internal/commands/tcp-routes"
	cmdTCPRoutesDelete "github.com/hashicorp/consul-api-gateway/internal/commands/tcp-routes/delete"
	cmdTCPRoutesGet "github.com/hashicorp/consul-api-gateway/internal/commands/tcp-routes/get"
	cmdTCPRoutesList "github.com/hashicorp/consul-api-gateway/internal/commands/tcp-routes/list"
	cmdTCPRoutesPut "github.com/hashicorp/consul-api-gateway/internal/commands/tcp-routes/put"
	cmdVersion "github.com/hashicorp/consul-api-gateway/internal/commands/version"

	"github.com/hashicorp/consul-api-gateway/internal/version"
)

func main() {
	ui := &cli.BasicUi{Writer: os.Stdout, ErrorWriter: os.Stderr}
	os.Exit(run(os.Args[1:], ui, os.Stdout))
}

func run(args []string, ui cli.Ui, logOutput io.Writer) int {
	c := cli.NewCLI("consul-api-gateway", version.GetHumanVersion())
	c.Args = args
	c.Commands = initializeCommands(ui, logOutput)
	c.HelpFunc = helpFunc(c.Commands)
	c.HelpWriter = logOutput

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}
	return exitStatus
}

func initializeCommands(ui cli.Ui, logOutput io.Writer) map[string]cli.CommandFactory {
	return map[string]cli.CommandFactory{
		"server": func() (cli.Command, error) {
			return cmdServer.New(context.Background(), ui, logOutput), nil
		},
		"exec": func() (cli.Command, error) {
			return cmdExec.New(context.Background(), ui, logOutput), nil
		},
		"version": func() (cli.Command, error) {
			return &cmdVersion.Command{UI: ui, Version: version.GetHumanVersion()}, nil
		},
		// gateway CRUD
		"gateways": func() (cli.Command, error) {
			return cmdGateways.New(), nil
		},
		"gateways delete": func() (cli.Command, error) {
			return cmdGatewaysDelete.New(context.Background(), ui, logOutput), nil
		},
		"gateways get": func() (cli.Command, error) {
			return cmdGatewaysGet.New(context.Background(), ui, logOutput), nil
		},
		"gateways list": func() (cli.Command, error) {
			return cmdGatewaysList.New(context.Background(), ui, logOutput), nil
		},
		"gateways put": func() (cli.Command, error) {
			return cmdGatewaysPut.New(context.Background(), ui, logOutput), nil
		},
		// http-route CRUD
		"http-routes": func() (cli.Command, error) {
			return cmdHTTPRoutes.New(), nil
		},
		"http-routes delete": func() (cli.Command, error) {
			return cmdHTTPRoutesDelete.New(context.Background(), ui, logOutput), nil
		},
		"http-routes get": func() (cli.Command, error) {
			return cmdHTTPRoutesGet.New(context.Background(), ui, logOutput), nil
		},
		"http-routes list": func() (cli.Command, error) {
			return cmdHTTPRoutesList.New(context.Background(), ui, logOutput), nil
		},
		"http-routes put": func() (cli.Command, error) {
			return cmdHTTPRoutesPut.New(context.Background(), ui, logOutput), nil
		},
		// tcp-route CRUD
		"tcp-routes": func() (cli.Command, error) {
			return cmdTCPRoutes.New(), nil
		},
		"tcp-routes delete": func() (cli.Command, error) {
			return cmdTCPRoutesDelete.New(context.Background(), ui, logOutput), nil
		},
		"tcp-routes get": func() (cli.Command, error) {
			return cmdTCPRoutesGet.New(context.Background(), ui, logOutput), nil
		},
		"tcp-routes list": func() (cli.Command, error) {
			return cmdTCPRoutesList.New(context.Background(), ui, logOutput), nil
		},
		"tcp-routes put": func() (cli.Command, error) {
			return cmdTCPRoutesPut.New(context.Background(), ui, logOutput), nil
		},
	}
}

func helpFunc(commands map[string]cli.CommandFactory) cli.HelpFunc {
	// This should be updated for any commands we want to hide for any reason.
	// Hidden commands can still be executed if you know the command, but
	// aren't shown in any help output. We use this for prerelease functionality
	// or advanced features.
	hidden := map[string]struct{}{
		"exec":   {},
		"server": {},
	}

	var include []string
	for k := range commands {
		if _, ok := hidden[k]; !ok {
			include = append(include, k)
		}
	}

	return cli.FilteredHelpFunc(include, cli.BasicHelpFunc("consul-api-gateway"))
}
