package server

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mitchellh/cli"
	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/polar/internal/consul"
	"github.com/hashicorp/polar/internal/envoy"
	"github.com/hashicorp/polar/k8s"
)

const (
	defaultSDSServerHost = "polar-controller.default.svc.cluster.local"
	defaultSDSServerPort = 9090
	// The amount of time to wait for the first cert write
	defaultCertWaitTime = 1 * time.Minute
)

type Command struct {
	UI     cli.Ui
	logger hclog.Logger

	flagCAFile            string // CA File for CA for Consul server
	flagCASecret          string // CA Secret for Consul server
	flagCASecretNamespace string // CA Secret namespace for Consul server
	flagConsulAddress     string // Consul server address
	flagSDSServerHost     string // SDS server host
	flagSDSServerPort     int    // SDS server port

	flagSet *flag.FlagSet
	once    sync.Once
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	c.flagSet.StringVar(&c.flagCAFile, "ca-file", "", "Path to CA for Consul server.")
	c.flagSet.StringVar(&c.flagCASecret, "ca-secret", "", "CA Secret for Consul server.")
	c.flagSet.StringVar(&c.flagCASecretNamespace, "ca-secret-namespace", "", "CA Secret namespace for Consul server.")
	c.flagSet.StringVar(&c.flagConsulAddress, "consul-address", "", "Consul Address.")
	c.flagSet.StringVar(&c.flagSDSServerHost, "sds-server-host", defaultSDSServerHost, "SDS Server Host.")
	c.flagSet.IntVar(&c.flagSDSServerPort, "sds-server-port", defaultSDSServerPort, "SDS Server Port.")
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)

	// Set up signal handlers and global context
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(interrupt)
		cancel()
	}()
	go func() {
		select {
		case <-interrupt:
			c.logger.Debug("received shutdown signal")
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := c.flagSet.Parse(args); err != nil {
		return 1
	}

	c.logger = hclog.Default().Named("polar-server")
	c.logger.SetLevel(hclog.Trace)

	consulCfg := api.DefaultConfig()
	cfg := k8s.Defaults()

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

	directory, err := os.MkdirTemp("", "polar-controller")
	if err != nil {
		c.logger.Error("error making temporary directory", "error", err)
		return 1
	}
	options := consul.DefaultCertManagerOptions()
	options.Directory = directory
	certManager := consul.NewCertManager(
		c.logger.Named("cert-manager"),
		consulClient,
		"polar-controller",
		options,
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		c.logger.Debug("running cert manager")
		return certManager.Manage(groupCtx)
	})

	// wait until we've written once before booting envoy
	waitCtx, waitCancel := context.WithTimeout(ctx, defaultCertWaitTime)
	defer waitCancel()
	c.logger.Debug("waiting for initial certs to be written")
	if err := certManager.WaitForWrite(waitCtx); err != nil {
		c.logger.Error("timeout waiting for certs to be written", "error", err)
		return 1
	}

	server := envoy.NewSDSServer(c.logger.Named("sds-server"), certManager)
	group.Go(func() error {
		c.logger.Debug("running sds-server")
		return server.Run(groupCtx)
	})
	group.Go(func() error {
		c.logger.Debug("running controller")
		return controller.Start(groupCtx)
	})

	if err := group.Wait(); err != nil {
		c.logger.Error("unexpected error", "error", err)
		return 1
	}

	c.logger.Info("shutting down")
	return 0
}

func (c *Command) Synopsis() string {
	return "Starts the polar control plane server"
}

func (c *Command) Help() string {
	return ""
}
