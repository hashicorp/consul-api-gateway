package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/mitchellh/cli"

	cmdExec "github.com/hashicorp/consul-api-gateway/internal/commands/exec"
	cmdServer "github.com/hashicorp/consul-api-gateway/internal/commands/server"
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
	}
}

func helpFunc(commands map[string]cli.CommandFactory) cli.HelpFunc {
	// This should be updated for any commands we want to hide for any reason.
	// Hidden commands can still be executed if you know the command, but
	// aren't shown in any help output. We use this for prerelease functionality
	// or advanced features.
	hidden := map[string]struct{}{
		"exec": {},
	}

	var include []string
	for k := range commands {
		if _, ok := hidden[k]; !ok {
			include = append(include, k)
		}
	}

	return cli.FilteredHelpFunc(include, cli.BasicHelpFunc("consul-api-gateway"))
}
