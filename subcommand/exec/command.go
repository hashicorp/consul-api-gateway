package exec

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/cli"
	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/polar/internal/consul"
	"github.com/hashicorp/polar/internal/envoy"
	"github.com/hashicorp/polar/internal/metrics"
)

// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/container_init.go#L272-L306
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/envoy_sidecar.go#L79
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/subcommand/connect-init/command.go#L91
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/endpoints_controller.go#L403

const (
	defaultBearerTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	// The amount of time to wait for the first cert write
	defaultCertWaitTime = 1 * time.Minute
)

type Command struct {
	UI cli.Ui

	// Consul params
	flagConsulHTTPAddress string // Address for Consul HTTP API.
	flagConsulHTTPPort    int    // Port for Consul HTTP communication
	flagConsulCACertFile  string // Root CA file for Consul
	flagConsulXDSPort     int    // Port for Consul xDS communication

	// Gateway params
	flagGatewayID        string // Gateway iD.
	flagGatewayHost      string // Gateway host.
	flagGatewayName      string // Gateway name.
	flagGatewayNamespace string // Gateway namespace.

	// Envoy params
	flagBootstrapPath    string // Path for config file for bootstrapping envoy
	flagSDSServerAddress string // Address for the SDS server
	flagSDSServerPort    int    // Port for the SDS server

	// ACL Auth
	flagACLAuthMethod       string // Auth Method to use for ACLs, if enabled.
	flagAuthMethodNamespace string // Consul namespace the auth-method is defined in.
	flagBearerTokenFile     string // Location of the bearer token. Default is /var/run/secrets/kubernetes.io/serviceaccount/token.

	// Logging
	flagLogLevel string
	flagLogJSON  bool

	flagSet *flag.FlagSet

	logger hclog.Logger

	once sync.Once
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	{
		// Consul
		c.flagSet.StringVar(&c.flagConsulHTTPAddress, "consul-http-address", "", "Address of Consul.")
		c.flagSet.IntVar(&c.flagConsulHTTPPort, "consul-http-port", 8500, "Port of Consul HTTP server.")
		c.flagSet.IntVar(&c.flagConsulXDSPort, "consul-xds-port", 8502, "Port of Consul xDS server.")
		c.flagSet.StringVar(&c.flagConsulCACertFile, "consul-ca-cert-file", "", "CA Root file for Consul.")
	}
	{
		// Envoy
		c.flagSet.StringVar(&c.flagBootstrapPath, "envoy-bootstrap-path", "", "Path to the config file for bootstrapping Envoy.")
		c.flagSet.StringVar(&c.flagSDSServerAddress, "envoy-sds-address", "", "Address of the SDS server.")
		c.flagSet.IntVar(&c.flagSDSServerPort, "envoy-sds-port", 9090, "Port of the SDS server.")
	}
	{
		// Gateway
		c.flagSet.StringVar(&c.flagGatewayID, "gateway-id", "", "ID of the gateway.")
		c.flagSet.StringVar(&c.flagGatewayHost, "gateway-host", "", "Host of the gateway.")
		c.flagSet.StringVar(&c.flagGatewayName, "gateway-name", "", "Name of the gateway.")
		c.flagSet.StringVar(&c.flagGatewayNamespace, "gateway-namespace", "default", "Name of the gateway namespace.")
	}
	{
		// ACL Auth
		c.flagSet.StringVar(&c.flagACLAuthMethod, "acl-auth-method", "", "Name of the auth method to login with.")
		c.flagSet.StringVar(&c.flagAuthMethodNamespace, "acl-auth-method-namespace", "default", "Consul namespace the auth-method is defined in")
		c.flagSet.StringVar(&c.flagBearerTokenFile, "acl-bearer-token-file", defaultBearerTokenFile, "Location of the bearer token.")
	}
	{
		// Logging
		c.flagSet.StringVar(&c.flagLogLevel, "log-level", "info",
			"Log verbosity level. Supported values (in order of detail) are \"trace\", "+
				"\"debug\", \"info\", \"warn\", and \"error\".")
		c.flagSet.BoolVar(&c.flagLogJSON, "log-json", false,
			"Enable or disable JSON output format for logging.")
	}
}

func (c *Command) Run(args []string) (ret int) {
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
		c.UI.Error("There was an error parsing the command line flags:\n\t" + err.Error())
		return 1
	}

	if c.logger == nil {
		c.logger = hclog.New(&hclog.LoggerOptions{
			Level:      hclog.LevelFromString(c.flagLogLevel),
			Output:     os.Stdout,
			JSONFormat: c.flagLogJSON,
		}).Named("polar-exec")
	}

	if err := c.validateFlags(); err != nil {
		c.logger.Error("invalid flags", "error", err)
		return 1
	}

	hostPort := fmt.Sprintf("%s:%d", c.flagConsulHTTPAddress, c.flagConsulHTTPPort)
	cfg := api.DefaultConfig()
	cfg.Address = hostPort
	if c.flagConsulCACertFile != "" {
		cfg.Scheme = "https"
		cfg.TLSConfig.CAFile = c.flagConsulCACertFile
	}
	consulClient, err := api.NewClient(cfg)
	if err != nil {
		c.logger.Error("error creating consul client", "error", err)
		return 1
	}

	// First do the ACL Login, if necessary.
	var token string
	if c.flagACLAuthMethod != "" {
		c.logger.Trace("logging in to consul")
		consulClient, token, err = c.login(ctx, consulClient, cfg)
		if err != nil {
			c.logger.Error("error logging into consul", "error", err)
			return 1
		}
		c.logger.Trace("consul login complete")
	}

	registry := consul.NewServiceRegistry(
		c.logger.Named("service-registry"),
		consulClient,
		c.flagGatewayName,
		c.flagGatewayNamespace,
		c.flagGatewayHost,
	)

	c.logger.Debug("registering service")
	if err := registry.Register(ctx); err != nil {
		c.logger.Error("error registering service", "error", err)
		return 1
	}
	defer func() {
		c.logger.Debug("deregistering service")
		// using context.Background here since the global context has
		// already been canceled at this point and we're just in a cleanup
		// function
		if err := registry.Deregister(context.Background()); err != nil {
			c.logger.Error("error deregistering service", "error", err)
			ret = 1
		}
	}()

	envoyManager := envoy.NewManager(
		c.logger.Named("envoy-manager"),
		envoy.ManagerConfig{
			ID:                registry.ID(),
			ConsulCA:          c.flagConsulCACertFile,
			ConsulAddress:     c.flagConsulHTTPAddress,
			ConsulXDSPort:     c.flagConsulXDSPort,
			BootstrapFilePath: c.flagBootstrapPath,
			LogLevel:          c.flagLogLevel,
			Token:             token,
		},
	)
	options := consul.DefaultCertManagerOptions()
	options.SDSAddress = c.flagSDSServerAddress
	options.SDSPort = c.flagSDSServerPort
	certManager := consul.NewCertManager(
		c.logger.Named("cert-manager"),
		metrics.Registry.Consul,
		consulClient,
		c.flagGatewayName,
		options,
	)
	sdsConfig, err := certManager.RenderSDSConfig()
	if err != nil {
		c.logger.Error("error rendering SDS configuration files", "error", err)
		return 1
	}
	err = envoyManager.RenderBootstrap(sdsConfig)
	if err != nil {
		c.logger.Error("error rendering Envoy configuration file", "error", err)
		return 1
	}

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

	group.Go(func() error {
		return envoyManager.Run(ctx)
	})

	c.logger.Info("started polar api gateway")
	if err := group.Wait(); err != nil {
		c.logger.Error("unexpected error", "error", err)
		return 1
	}

	c.logger.Info("shutting down")
	return 0
}

func (c *Command) validateFlags() error {
	if c.flagConsulHTTPAddress == "" {
		return errors.New("-consul-http-address must be set")
	}
	if c.flagGatewayHost == "" {
		return errors.New("-gateway-host must be set")
	}
	if c.flagGatewayName == "" {
		return errors.New("-gateway-name must be set")
	}
	if c.flagBootstrapPath == "" {
		return errors.New("-envoy-bootstrap-path must be set")
	}
	if c.flagSDSServerAddress == "" {
		return errors.New("-envoy-sds-address must be set")
	}
	if c.flagGatewayID == "" {
		c.flagGatewayID = uuid.New().String()
	}
	return nil
}

func (c *Command) login(ctx context.Context, client *api.Client, cfg *api.Config) (*api.Client, string, error) {
	data, err := os.ReadFile(c.flagBearerTokenFile)
	if err != nil {
		return nil, "", fmt.Errorf("error reading bearer token: %w", err)
	}
	bearerToken := strings.TrimSpace(string(data))

	token, err := consul.NewAuthenticator(
		c.logger.Named("authenticator"),
		client,
		c.flagACLAuthMethod,
		c.flagAuthMethodNamespace,
	).Authenticate(ctx, c.flagGatewayName, bearerToken)

	if err != nil {
		return nil, "", fmt.Errorf("error logging in to consul: %w", err)
	}

	// Now update the client so that it will read the ACL token we just fetched.
	cfg.Token = token
	newClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("error updating client connection with token: %w", err)
	}
	return newClient, token, nil
}

func (c *Command) Synopsis() string {
	return "Polar exec command"
}

func (c *Command) Help() string {
	return `
Usage: polar exec [options]

	Handles service registration, certificate rotation, and spawning envoy.
`
}
