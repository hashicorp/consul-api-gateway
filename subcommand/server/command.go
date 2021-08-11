package server

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mitchellh/cli"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/k8s"
)

type Command struct {
	UI     cli.Ui
	logger hclog.Logger
}

func (c *Command) Run(_ []string) int {
	c.logger = hclog.Default().Named("polar-server")
	c.logger.SetLevel(hclog.Trace)

	consul, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		c.UI.Error("An error occurred creating a Consul API client:\n\t" + err.Error())
		return 1
	}

	controller, err := k8s.New(consul, c.logger, nil)
	if err != nil {
		c.UI.Error("An error occurred creating the kubernetes controller:\n\t" + err.Error())
		return 1
	}

	// wait for signal
	signalCh := make(chan os.Signal, 10)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := controller.Start(ctx); err != nil {
		c.UI.Error("An error occurred starting the kubernetes controller:\n\t" + err.Error())
	}

	for {
		select {
		case sig := <-signalCh:
			c.logger.Info("Caught", "signal", sig)
			c.logger.Info("Shutting down server...")
			return 0
		case <-controller.Failed():
			return 1
		}
	}
}

func (c *Command) Synopsis() string {
	return "Starts the polar control plane server"
}

func (c *Command) Help() string {
	return ""
}
