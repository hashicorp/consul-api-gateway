package main

import (
	"github.com/mitchellh/cli"

	cmdExec "github.com/hashicorp/consul-api-gateway/internal/commands/exec"
	cmdServer "github.com/hashicorp/consul-api-gateway/internal/commands/server"
	cmdVersion "github.com/hashicorp/consul-api-gateway/internal/commands/version"
	"github.com/hashicorp/consul-api-gateway/internal/version"
)

func initializeCommands(ui cli.Ui) map[string]cli.CommandFactory {
	return map[string]cli.CommandFactory{
		"server": func() (cli.Command, error) {
			return &cmdServer.Command{UI: ui}, nil
		},
		"exec": func() (cli.Command, error) {
			return &cmdExec.Command{UI: ui}, nil
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
