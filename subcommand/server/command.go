package server

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/mitchellh/cli"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/k8s"
)

type Command struct {
	UI     cli.Ui
	logger hclog.Logger

	flagCASecret      string // CA Secret for Consul server
	flagConsulAddress string // Consul server address

	flagSet *flag.FlagSet
	once    sync.Once
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	c.flagSet.StringVar(&c.flagCASecret, "ca-secret", "", "CA Secret for Consul server.")
	c.flagSet.StringVar(&c.flagConsulAddress, "consul-address", "", "Consul Address.")
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)

	if err := c.flagSet.Parse(args); err != nil {
		return 1
	}

	c.logger = hclog.Default().Named("polar-server")
	c.logger.SetLevel(hclog.Trace)

	consulCfg := api.DefaultConfig()
	cfg := k8s.Defaults()

	file, err := ioutil.TempFile("", "polar")
	if c.flagCASecret != "" {
		consulCfg.TLSConfig.CAFile = file.Name()
		cfg.CACertFile = file.Name()
		cfg.CACertSecret = c.flagCASecret
	}
	defer os.Remove(file.Name())

	if c.flagConsulAddress != "" {
		consulCfg.Address = c.flagConsulAddress
	}

	controller, err := k8s.New(c.logger, cfg)
	if err != nil {
		c.UI.Error("An error occurred creating the kubernetes controller:\n\t" + err.Error())
		return 1
	}

	consul, err := api.NewClient(consulCfg)
	if err != nil {
		c.UI.Error("An error occurred creating a Consul API client:\n\t" + err.Error())
		return 1
	}

	controller.SetConsul(consul)

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
