package main

import (
	"log"
	"os"

	"github.com/mitchellh/cli"

	"github.com/hashicorp/polar/version"
)

func main() {
	c := cli.NewCLI("polar", version.GetHumanVersion())
	c.Args = os.Args[1:]
	c.Commands = Commands
	c.HelpFunc = helpFunc()

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}
	os.Exit(exitStatus)
}
