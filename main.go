package main

import (
	"log"
	"os"

	"github.com/mitchellh/cli"

	"github.com/hashicorp/consul-api-gateway/internal/version"
)

func main() {
	ui := &cli.BasicUi{Writer: os.Stdout, ErrorWriter: os.Stderr}
	os.Exit(run(os.Args[1:], ui))
}

func run(args []string, ui cli.Ui) int {
	c := cli.NewCLI("consul-api-gateway", version.GetHumanVersion())
	c.Args = args
	c.Commands = initializeCommands(ui)
	c.HelpFunc = helpFunc(c.Commands)

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}
	return exitStatus
}
