package sdsserver

import (
	"context"
	"flag"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/sds"

	"github.com/mitchellh/cli"
)

type cmd struct {
	UI         cli.Ui
	shutdownCh <-chan struct{}

	flags      *flag.FlagSet
	addr       string
	secretFile string
}

func New(ui cli.Ui, shutdownCh <-chan struct{}) cli.Command {
	c := &cmd{UI: ui, shutdownCh: shutdownCh}
	c.init()
	return c
}

func (c *cmd) init() {
	fs := flag.NewFlagSet("SDS Server", flag.ContinueOnError)
	fs.StringVar(&c.addr, "addr", ":9090", "address to listen for SDS gRPC requests on. Defaults to `:9090`.")
	fs.StringVar(&c.secretFile, "static-secrets", "", "path to a JSON file containing static secrets to serve.")
	c.flags = fs
}

func (c *cmd) Run(args []string) int {
	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	// Run SDS server until shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel the context if the shutdownCh closes TODO should we use context
	// instead? It's weird if we do that we save a context rather than pass it
	// through Run call but that is a bigger CLI change.
	go func() {
		<-c.shutdownCh
		cancel()
	}()

	// TODO maybe move this somewhere we can share it for all subcommands with a
	// flag to set level.
	log := hclog.New(&hclog.LoggerOptions{
		Name:  "polar",
		Level: hclog.LevelFromString("TRACE"),
	})
	s := sds.NewServer(c.addr, log)

	if c.secretFile != "" {
		if err := c.loadStaticSecrets(s); err != nil {
			c.UI.Error(fmt.Sprintf("Failed loading secrets from file %s: %s", c.secretFile, err))
			return 1
		}
	}

	c.UI.Info(fmt.Sprintf("Starting SDS server on: %s", c.addr))

	if err := s.Serve(ctx); err != nil {
		c.UI.Error(fmt.Sprintf("SDS server terminated with error: %s", err))
		return 1
	}

	return 0
}

func (c *cmd) Synopsis() string {
	return "Runs the SDS server standalone"
}

func (c *cmd) Help() string {
	return "polar sds-server -addr :9090"
}
