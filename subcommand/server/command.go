package server

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/hashicorp/polar/internal/metrics"
	"github.com/hashicorp/polar/internal/profiling"
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
	flagMetricsPort       int    // Port for prometheus metrics
	flagPprofPort         int    // Port for pprof profiling

	// Logging
	flagLogLevel string
	flagLogJSON  bool

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
	c.flagSet.IntVar(&c.flagMetricsPort, "metrics-port", 0, "Metrics port, if not set, metrics are not enabled.")
	c.flagSet.IntVar(&c.flagPprofPort, "pprof-port", 0, "Go pprof port, if not set, profiling is not enabled.")
	{
		// Logging
		c.flagSet.StringVar(&c.flagLogLevel, "log-level", "info",
			"Log verbosity level. Supported values (in order of detail) are \"trace\", "+
				"\"debug\", \"info\", \"warn\", and \"error\".")
		c.flagSet.BoolVar(&c.flagLogJSON, "log-json", false,
			"Enable or disable JSON output format for logging.")
	}
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
	metricsRegistry := metrics.Registry

	if c.logger == nil {
		c.logger = hclog.New(&hclog.LoggerOptions{
			Level:      hclog.LevelFromString(c.flagLogLevel),
			Output:     os.Stdout,
			JSONFormat: c.flagLogJSON,
		}).Named("polar-server")
	}

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
			c.logger.Error("error creating the kubernetes controller", "error", err)
			return 1
		}
		defer os.Remove(file.Name())
		cfg.CACertFile = file.Name()
		consulCfg.TLSConfig.CAFile = file.Name()
	}

	if c.flagConsulAddress != "" {
		consulCfg.Address = c.flagConsulAddress
	}

	secretFetcher, err := k8s.NewK8sSecretClient(c.logger.Named("cert-fetcher"), metricsRegistry.SDS)
	if err != nil {
		c.logger.Error("error initializing the kubernetes secret fetcher", "error", err)
		return 1
	}

	controller, err := k8s.New(c.logger, metricsRegistry.K8s, cfg)
	if err != nil {
		c.logger.Error("error creating the kubernetes controller", "error", err)
		return 1
	}

	consulClient, err := api.NewClient(consulCfg)
	if err != nil {
		c.logger.Error("error creating a Consul API client", "error", err)
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
		return certManager.Manage(groupCtx)
	})

	// wait until we've written once before booting envoy
	waitCtx, waitCancel := context.WithTimeout(ctx, defaultCertWaitTime)
	defer waitCancel()
	c.logger.Trace("waiting for initial certs to be written")
	if err := certManager.WaitForWrite(waitCtx); err != nil {
		c.logger.Error("timeout waiting for certs to be written", "error", err)
		return 1
	}
	c.logger.Trace("initial certificates written")

	server := envoy.NewSDSServer(c.logger.Named("sds-server"), metricsRegistry.SDS, certManager, secretFetcher)
	group.Go(func() error {
		return server.Run(groupCtx)
	})
	group.Go(func() error {
		return controller.Start(groupCtx)
	})

	if c.flagMetricsPort != 0 {
		group.Go(func() error {
			return metrics.RunServer(groupCtx, c.logger.Named("metrics"), fmt.Sprintf("127.0.0.1:%d", c.flagMetricsPort))
		})
	}

	if c.flagPprofPort != 0 {
		group.Go(func() error {
			return profiling.RunServer(groupCtx, c.logger.Named("pprof"), fmt.Sprintf("127.0.0.1:%d", c.flagPprofPort))
		})
	}

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
