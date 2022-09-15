package controller

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	consulAdapters "github.com/hashicorp/consul-api-gateway/internal/adapters/consul"
	"github.com/hashicorp/consul-api-gateway/internal/api"
	"github.com/hashicorp/consul-api-gateway/internal/api/apiinternal"
	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	commonCLI "github.com/hashicorp/consul-api-gateway/internal/cli"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul-api-gateway/internal/profiling"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/consul-api-gateway/internal/vault"
	"github.com/hashicorp/consul-api-gateway/internal/vm"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/mitchellh/cli"
	"golang.org/x/sync/errgroup"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["controller"] = func() (cli.Command, error) {
		return NewCommand(ctx, ui, logOutput), nil
	}
}

type Command struct {
	*commonCLI.CommonCLI
	help string

	flagControllerAddress  string // Server address for requests
	flagControllerPort     uint   // Server port for requests
	flagControllerCertFile string // Server TLS certificate file
	flagControllerKeyFile  string // Server TLS key file

	flagConsulAddress           string // Consul server address
	flagConsulXDSPort           uint   // Consul xDS server port
	flagConsulToken             string // Consul token
	flagConsulScheme            string // Consul server scheme
	flagConsulTLSCAFile         string // Consul TLS CA file for TLS verification
	flagConsulTLSClientCertFile string // Consul client mTLS certificate
	flagConsulTLSClientKeyFile  string // Consul client mTLS key
	flagConsulTLSSkipVerify     bool   // Skip Consul TLS verification

	flagConsulRegistrationName      string              // Name of service to register in Consul
	flagConsulRegistrationNamespace string              // Namespace of service to register in Consul
	flagConsulRegistrationTags      commonCLI.ArrayFlag // Tags for service to register in Consul

	flagVaultAddress string // Vault address to use
	flagVaultToken   string // Vault token to use
	flagVaultMount   string // Vault KV mount to use

	flagConsulStoragePath      string // Storage path for persistent data
	flagConsulStorageNamespace string // Storage namespace for persistent data

	flagSDSAddress string // Server address to use for SDS
	flagSDSPort    uint   // Server port to use for SDS

	flagDebugProfilingPort uint // Port for pprof profiling
	flagDebugMetricsPort   uint // Port for Prometheus metrics
}

func NewCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command {
	cmd := &Command{
		CommonCLI: commonCLI.NewCommonCLI(ctx, help, synopsis, ui, logOutput, "controller"),
	}
	cmd.init()
	cmd.help = commonCLI.FlagUsage(help, cmd.Flags)

	return cmd
}

func (c *Command) init() {
	c.Flags.StringVar(&c.flagControllerAddress, "gateway-controller-address", "localhost", "Server address to use for client.")
	c.Flags.UintVar(&c.flagControllerPort, "gateway-controller-port", 5605, "Server port to use for client.")
	c.Flags.StringVar(&c.flagControllerCertFile, "gateway-controller-cert-file", "", "Path to TLS certificate file for HTTPS connections.")
	c.Flags.StringVar(&c.flagControllerKeyFile, "gateway-controller-key-file", "", "Path to TLS key file for HTTPS connections.")

	c.Flags.StringVar(&c.flagConsulAddress, "consul-address", "127.0.0.1:8500", "Consul Address.")
	c.Flags.UintVar(&c.flagConsulXDSPort, "consul-xds-port", 8502, "Consul xDS port.")
	c.Flags.StringVar(&c.flagConsulToken, "consul-token", "", "Token to use for Consul client.")
	c.Flags.StringVar(&c.flagConsulScheme, "consul-scheme", "", "Scheme to use for Consul client.")
	c.Flags.StringVar(&c.flagConsulTLSCAFile, "consul-tls-ca-file", "", "Path to CA for Consul server.")
	c.Flags.StringVar(&c.flagConsulTLSClientCertFile, "consul-tls-client-cert-file", "", "Path to client certificate file for Consul.")
	c.Flags.StringVar(&c.flagConsulTLSClientKeyFile, "consul-tls-client-key-file", "", "Path to client key file for Consul.")
	c.Flags.BoolVar(&c.flagConsulTLSSkipVerify, "consul-tls-skip-verify", false, "Skip verification for Consul connection.")

	c.Flags.StringVar(&c.flagConsulRegistrationNamespace, "consul-registration-namespace", "", "Namespace to use for Consul service registration.")
	c.Flags.StringVar(&c.flagConsulRegistrationName, "consul-registration-name", "api-gateway-controller", "Name to use for Consul service registration.")
	c.Flags.Var(&c.flagConsulRegistrationTags, "consul-registration-tags", "Tags to add for Consul service registration.")

	c.Flags.StringVar(&c.flagVaultAddress, "vault-address", "", "Vault address to use.")
	c.Flags.StringVar(&c.flagVaultToken, "vault-token", "", "Vault token to use.")
	c.Flags.StringVar(&c.flagVaultMount, "vault-mount", "", "Vault KV mount to use.")

	c.Flags.StringVar(&c.flagConsulStoragePath, "consul-storage-path", "", "Storage path for Gateway data persisted in Consul.")
	c.Flags.StringVar(&c.flagConsulStorageNamespace, "consul-storage-namespace", "", "Storage namespace for Gateway data persisted in Consul.")

	c.Flags.StringVar(&c.flagSDSAddress, "sds-address", "", "Server address to use for SDS.")
	c.Flags.UintVar(&c.flagSDSPort, "sds-port", 5606, "Server port to use for SDS.")

	c.Flags.UintVar(&c.flagDebugMetricsPort, "debug-metrics-port", 5607, "Server port to use for metrics.")
	c.Flags.UintVar(&c.flagDebugProfilingPort, "debug-pprof-port", 5608, "Server port to use for pprof debugging.")
}

func (c *Command) Run(args []string) (ret int) {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	logger := c.Logger("controller")
	address := fmt.Sprintf("%s:%d", c.flagControllerAddress, c.flagControllerPort)

	vault, err := vaultapi.NewClient(&vaultapi.Config{
		Address: c.flagVaultAddress,
	})
	if err != nil {
		return c.Error("initializing Vault client", err)
	}
	vault.SetToken(c.flagVaultToken)
	vaultKVClient := vault.KVv2(c.flagVaultMount)

	client, err := consulapi.NewClient(c.ConsulConfig())
	if err != nil {
		return c.Error("initializing Consul client", err)
	}

	secretClient, err := registerSecretClients(logger)
	if err != nil {
		return c.Error("initializing secret fetchers", err)
	}

	ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	group, groupCtx := errgroup.WithContext(ctx)

	certManager := consul.NewCertManager(
		logger.Named("cert-manager"),
		client,
		c.flagConsulRegistrationName,
		consul.DefaultCertManagerOptions(),
	)
	group.Go(func() error {
		return certManager.Manage(groupCtx)
	})

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()

	if err := certManager.WaitForWrite(waitCtx); err != nil {
		return c.Error("timeout waiting for certs to be written", err)
	}

	backend := store.NewConsulBackend(c.flagConsulRegistrationName, client, c.flagConsulStorageNamespace, c.flagConsulStoragePath)
	// replace this with the Consul store
	store := store.New(store.Config{
		Adapter:   consulAdapters.NewSyncAdapter(logger.Named("consul-adapter"), client),
		Backend:   backend,
		Binder:    &v1.Binder{},
		Logger:    logger.Named("store"),
		Marshaler: &v1.Marshaler{},
	})

	sds := envoy.NewSDSServer(
		logger.Named("sds-server"),
		certManager,
		secretClient,
		store,
	).WithAddress(c.flagSDSAddress, c.flagSDSPort)
	group.Go(func() error {
		return sds.Run(groupCtx)
	})

	server := api.NewServer(api.ServerConfig{
		Logger:          logger,
		Consul:          client,
		Address:         address,
		CertFile:        c.flagControllerCertFile,
		KeyFile:         c.flagControllerKeyFile,
		Name:            c.flagConsulRegistrationName,
		Namespace:       c.flagConsulRegistrationNamespace,
		ShutdownTimeout: 10 * time.Second,
		Validator:       vm.NewValidator(logger.Named("validator"), vaultKVClient, client),
		Store:           store,
		Bootstrap: apiinternal.BootstrapConfiguration{
			Consul: apiinternal.ConsulConfiguration{
				Server:  c.flagConsulAddress,
				XdsPort: int(c.flagConsulXDSPort),
			},
			SdsPort: int(c.flagSDSPort),
		},
	})
	group.Go(func() error {
		return server.Run(groupCtx)
	})

	if c.flagDebugMetricsPort != 0 {
		group.Go(func() error {
			return metrics.RunServer(groupCtx, logger.Named("metrics"), fmt.Sprintf("127.0.0.1:%d", c.flagDebugMetricsPort))
		})
	}

	// Run profiling server if configured
	if c.flagDebugProfilingPort != 0 {
		group.Go(func() error {
			return profiling.RunServer(groupCtx, logger.Named("pprof"), fmt.Sprintf("127.0.0.1:%d", c.flagDebugProfilingPort))
		})
	}

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

func registerSecretClients(logger hclog.Logger) (*envoy.MultiSecretClient, error) {
	secretClient := envoy.NewMultiSecretClient()

	vaultPKIClient, err := vault.NewPKISecretClient(logger.Named("vault-pki-cert-fetcher"), "pki", "TODO")
	if err != nil {
		logger.Error("error initializing the Vault PKI cert fetcher", "error", err)
		return nil, err
	}
	secretClient.Register(vault.PKISecretScheme, vaultPKIClient)

	vaultStaticClient, err := vault.NewKVSecretClient(logger.Named("vault-kv-cert-fetcher"), "secret")
	if err != nil {
		logger.Error("error initializing the Vault KV cert fetcher", "error", err)
		return nil, err
	}
	secretClient.Register(vault.KVSecretScheme, vaultStaticClient)

	return secretClient, nil
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
