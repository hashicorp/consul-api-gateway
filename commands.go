package main

import (
	"os"

	"github.com/mitchellh/cli"

	cmdServer "github.com/hashicorp/polar/subcommand/server"
	cmdVersion "github.com/hashicorp/polar/subcommand/version"
	"github.com/hashicorp/polar/version"
)

// Commands is the mapping of all available polar commands.
var Commands map[string]cli.CommandFactory

func init() {
	ui := &cli.BasicUi{Writer: os.Stdout, ErrorWriter: os.Stderr}

	Commands = map[string]cli.CommandFactory{
		"server": func() (cli.Command, error) {
			return &cmdServer.Command{UI: ui}, nil
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
		"inject-connect": struct{}{},
	}

	var include []string
	for k := range Commands {
		if _, ok := hidden[k]; !ok {
			include = append(include, k)
		}
	}

	return cli.FilteredHelpFunc(include, cli.BasicHelpFunc("polar"))
}
