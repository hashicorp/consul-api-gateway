package server

import (
	"context"
	"flag"
	"io/ioutil"
	"net"
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

	flagCAFile            string // CA File for CA for Consul server
	flagCASecret          string // CA Secret for Consul server
	flagCASecretNamespace string // CA Secret namespace for Consul server
	flagConsulAddress     string // Consul server address
	flagAddress           string // Server address

	flagSet *flag.FlagSet
	once    sync.Once
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	c.flagSet.StringVar(&c.flagCAFile, "ca-file", "", "Path to CA for Consul server.")
	c.flagSet.StringVar(&c.flagCASecret, "ca-secret", "", "CA Secret for Consul server.")
	c.flagSet.StringVar(&c.flagCASecretNamespace, "ca-secret-namespace", "", "CA Secret namespace for Consul server.")
	c.flagSet.StringVar(&c.flagConsulAddress, "consul-address", "", "Consul Address.")
	c.flagSet.StringVar(&c.flagAddress, "address", "", "Address for this server which can be injected into polar containers")
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

	if c.flagAddress == "" {
		address, err := defaultIP()
		if err != nil {
			c.UI.Error("An error occurred getting the default IP address of the server:\n\t" + err.Error())
			return 1
		}
		c.flagAddress = address
	}
	cfg.ServerAnnouncementAddress = c.flagAddress

	if c.flagCAFile != "" {
		consulCfg.TLSConfig.CAFile = c.flagCAFile
		cfg.CACertFile = c.flagCAFile
	}

	if c.flagCASecret != "" {
		cfg.CACertSecret = c.flagCASecret
		if c.flagCASecretNamespace != "" {
			cfg.CACertSecretNamespace = c.flagCASecretNamespace
		}

		// if we're pulling the cert from a secret, then we override the location
		// where we store it
		file, err := ioutil.TempFile("", "polar")
		if err != nil {
			c.UI.Error("An error occurred creating the kubernetes controller:\n\t" + err.Error())
			return 1
		}
		defer os.Remove(file.Name())
		cfg.CACertFile = file.Name()
		consulCfg.TLSConfig.CAFile = file.Name()
	}

	if c.flagConsulAddress != "" {
		consulCfg.Address = c.flagConsulAddress
	}

	controller, err := k8s.New(c.logger, cfg)
	if err != nil {
		c.UI.Error("An error occurred creating the kubernetes controller:\n\t" + err.Error())
		return 1
	}

	consulClient, err := api.NewClient(consulCfg)
	if err != nil {
		c.UI.Error("An error occurred creating a Consul API client:\n\t" + err.Error())
		return 1
	}

	controller.SetConsul(consulClient)

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

func defaultIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
