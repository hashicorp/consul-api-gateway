package controller

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/api"
	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/mitchellh/cli"
	"golang.org/x/sync/errgroup"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["controller"] = func() (cli.Command, error) {
		return NewCommand(ctx, ui, logOutput), nil
	}

	commands["controller health"] = func() (cli.Command, error) {
		return NewHealthCommand(ctx, ui, logOutput), nil
	}
}

type Command struct {
	*common.CommonCLI
	help string

	flagControllerAddress  string // Server address for requests
	flagControllerPort     uint   // Server port for requests
	flagControllerCertFile string // Server TLS certificate file
	flagControllerKeyFile  string // Server TLS key file

	flagConsulAddress           string // Consul server address
	flagConsulToken             string // Consul token
	flagConsulScheme            string // Consul server scheme
	flagConsulTLSCAFile         string // Consul TLS CA file for TLS verification
	flagConsulTLSClientCertFile string // Consul client mTLS certificate
	flagConsulTLSClientKeyFile  string // Consul client mTLS key
	flagConsulTLSSkipVerify     bool   // Skip Consul TLS verification

	flagConsulRegistrationName      string           // Name of service to register in Consul
	flagConsulRegistrationNamespace string           // Namespace of service to register in Consul
	flagConsulRegistrationTags      common.ArrayFlag // Tags for service to register in Consul

	flagConsulStoragePath      string // Storage path for persistent data
	flagConsulStorageNamespace string // Storage namespace for persistent data
}

func NewCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	cmd := &Command{
		CommonCLI: common.NewCommonCLI(ctx, help, synopsis, ui, logOutput, "controller"),
	}
	cmd.init()
	cmd.help = common.FlagUsage(help, cmd.Flags)

	return cmd
}

func (c *Command) init() {
	c.Flags.StringVar(&c.flagControllerAddress, "gateway-controller-address", "localhost", "Server address to use for client.")
	c.Flags.UintVar(&c.flagControllerPort, "gateway-controller-port", 5605, "Server port to use for client.")
	c.Flags.StringVar(&c.flagControllerCertFile, "gateway-controller-cert-file", "", "Path to TLS certificate file for HTTPS connections.")
	c.Flags.StringVar(&c.flagControllerKeyFile, "gateway-controller-key-file", "", "Path to TLS key file for HTTPS connections.")

	c.Flags.StringVar(&c.flagConsulAddress, "consul-address", "", "Consul Address.")
	c.Flags.StringVar(&c.flagConsulToken, "consul-token", "", "Token to use for Consul client.")
	c.Flags.StringVar(&c.flagConsulScheme, "consul-scheme", "", "Scheme to use for Consul client.")
	c.Flags.StringVar(&c.flagConsulTLSCAFile, "consul-tls-ca-file", "", "Path to CA for Consul server.")
	c.Flags.StringVar(&c.flagConsulTLSClientCertFile, "consul-tls-client-cert-file", "", "Path to client certificate file for Consul.")
	c.Flags.StringVar(&c.flagConsulTLSClientKeyFile, "consul-tls-client-key-file", "", "Path to client key file for Consul.")
	c.Flags.BoolVar(&c.flagConsulTLSSkipVerify, "consul-tls-skip-verify", false, "Skip verification for Consul connection.")

	c.Flags.StringVar(&c.flagConsulRegistrationNamespace, "consul-registration-namespace", "", "Namespace to use for Consul service registration.")
	c.Flags.StringVar(&c.flagConsulRegistrationName, "consul-registration-name", "api-gateway-controller", "Name to use for Consul service registration.")
	c.Flags.Var(&c.flagConsulRegistrationTags, "consul-registration-tags", "Tags to add for Consul service registration.")

	c.Flags.StringVar(&c.flagConsulStoragePath, "consul-storage-path", "", "Storage path for Gateway data persisted in Consul.")
	c.Flags.StringVar(&c.flagConsulStorageNamespace, "consul-storage-namespace", "", "Storage namespace for Gateway data persisted in Consul.")
}

func (c *Command) Run(args []string) (ret int) {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	logger := c.Logger("controller")
	address := fmt.Sprintf("%s:%d", c.flagControllerAddress, c.flagControllerPort)

	client, err := consulapi.NewClient(c.ConsulConfig())
	if err != nil {
		return c.Error("initializing Consul client", err)
	}

	ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	group, groupCtx := errgroup.WithContext(ctx)

	// add store stuff here

	server := api.NewServer(api.ServerConfig{
		Logger:          logger,
		Consul:          client,
		Address:         address,
		CertFile:        c.flagControllerCertFile,
		KeyFile:         c.flagControllerKeyFile,
		ShutdownTimeout: 10 * time.Second,
	})
	group.Go(func() error {
		return server.Run(groupCtx)
	})

	registry := consul.NewServiceRegistry(
		logger.Named("service-registry"),
		client,
		c.flagConsulRegistrationName,
		c.flagConsulRegistrationNamespace,
		c.flagControllerAddress,
	).WithTags(c.flagConsulRegistrationTags)

	if err := registry.Register(groupCtx); err != nil {
		return c.Error("error registering controller", err)
	}
	defer func() {
		logger.Trace("deregistering controller")
		// using context.Background here since the global context has
		// already been canceled at this point and we're just in a cleanup
		// function
		if err := registry.Deregister(context.Background()); err != nil {
			logger.Error("error deregistering service", "error", err)
			ret = 1
		}
	}()

	if err := group.Wait(); err != nil {
		return c.Error("unexpected error", err)
	}

	return c.Success("Stopping Gateway API controller")
}

func (c *Command) ConsulConfig() *consulapi.Config {
	consulCfg := consulapi.DefaultConfig()
	if c.flagConsulAddress != "" {
		consulCfg.Address = c.flagConsulAddress
	}
	if c.flagConsulToken != "" {
		consulCfg.Token = c.flagConsulToken
	}
	if c.flagConsulScheme != "" {
		consulCfg.Scheme = c.flagConsulScheme
	}
	if c.flagConsulTLSCAFile != "" {
		consulCfg.TLSConfig.CAFile = c.flagConsulTLSCAFile
	}
	if c.flagConsulTLSClientCertFile != "" {
		consulCfg.TLSConfig.CertFile = c.flagConsulTLSClientCertFile
	}
	if c.flagConsulTLSClientKeyFile != "" {
		consulCfg.TLSConfig.KeyFile = c.flagConsulTLSClientKeyFile
	}
	if c.flagConsulTLSSkipVerify {
		consulCfg.TLSConfig.InsecureSkipVerify = true
	}

	return consulCfg
}

func (c *Command) Help() string {
	return help
}

const synopsis = "Manage Consul API Gateway Controller"
const help = `
Usage: consul-api-gateway controller <subcommand> [options] [args]
  This command has subcommands for interacting with Consul API Gateway
  Controller. Here are some simple examples, and more detailed examples
	are available in the subcommands or the documentation.

  Checking Gateway controller health:

    $ consul-api-gateway controller health

  For more examples, ask for subcommand help or view the documentation at
  https://www.consul.io/docs/api-gateway.
`
