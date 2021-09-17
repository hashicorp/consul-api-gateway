package main

import (
	"os"

	"github.com/mitchellh/cli"

	cmdExec "github.com/hashicorp/consul-api-gateway/subcommand/exec"
	cmdServer "github.com/hashicorp/consul-api-gateway/subcommand/server"
	cmdVersion "github.com/hashicorp/consul-api-gateway/subcommand/version"
	"github.com/hashicorp/consul-api-gateway/version"
)

// Commands is the mapping of all available consul-api-gateway commands.
var Commands map[string]cli.CommandFactory

func init() {
	ui := &cli.BasicUi{Writer: os.Stdout, ErrorWriter: os.Stderr}

	Commands = map[string]cli.CommandFactory{
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

func helpFunc() cli.HelpFunc {
	// This should be updated for any commands we want to hide for any reason.
	// Hidden commands can still be executed if you know the command, but
	// aren't shown in any help output. We use this for prerelease functionality
	// or advanced features.
	hidden := map[string]struct{}{
		"exec": {},
	}

	var include []string
	for k := range Commands {
		if _, ok := hidden[k]; !ok {
			include = append(include, k)
		}
	}

	return cli.FilteredHelpFunc(include, cli.BasicHelpFunc("consul-api-gateway"))
}
