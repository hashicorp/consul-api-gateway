package deployment

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/api"
	"github.com/hashicorp/consul-api-gateway/internal/api/apiinternal"
	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	consulapi "github.com/hashicorp/consul/api"
	"golang.org/x/sync/errgroup"

	"github.com/mitchellh/cli"
)

func RegisterCommands(ctx context.Context, commands map[string]cli.CommandFactory, ui cli.Ui, logOutput io.Writer) {
	commands["deployment"] = func() (cli.Command, error) {
		return NewCommand(ctx, ui, logOutput), nil
	}
}

type Command struct {
	*common.CommonCLI
	help string

	flagServiceIP string // IP Address to register for the Gateway service

	flagAddress     string // Server address for requests
	flagPort        uint   // Server port for requests
	flagToken       string // Token for requests
	flagScheme      string // Server scheme for API
	flagCAFile      string // Server TLS CA file for TLS verification
	flagSkipVerify  bool   // Skip certificate verification for client
	flagEnvoyBinary string // Path to envoy binary
}

func NewCommand(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	cmd := &Command{
		CommonCLI: common.NewCommonCLI(ctx, help, synopsis, ui, logOutput, "deployment"),
	}
	cmd.init()
	cmd.help = common.FlagUsage(help, cmd.Flags)

	return cmd
}

func (c *Command) init() {
	c.Flags.StringVar(&c.flagServiceIP, "gateway-ip", "", "IP to register with Consul, if unspecified the IP is dynamically found via system routing tables.")
	c.Flags.StringVar(&c.flagToken, "gateway-controller-token", "", "Token to use for client.")
	c.Flags.StringVar(&c.flagAddress, "gateway-controller-address", "localhost", "Server address to use for client.")
	c.Flags.UintVar(&c.flagPort, "gateway-controller-port", 5605, "Server port to use for client.")
	c.Flags.StringVar(&c.flagScheme, "gateway-controller-scheme", "http", "Server scheme to use for client.")
	c.Flags.StringVar(&c.flagCAFile, "gateway-controller-ca-file", "", "Path to CA file for verifying server TLS certificate.")
	c.Flags.BoolVar(&c.flagSkipVerify, "gateway-controller-skip-verify", false, "Skip certificate verification for TLS connection.")
	c.Flags.StringVar(&c.flagEnvoyBinary, "envoy-binary", "", "Path to envoy binary.")
}

func (c *Command) Run(args []string) (ret int) {
	if err := c.Parse(args); err != nil {
		return c.Error("parsing command line flags", err)
	}

	// first check that we actually have envoy installed
	envoyBinary, err := getEnvoyBinary(c.flagEnvoyBinary)
	if err != nil {
		return c.Error("finding envoy binary", err)
	}

	logger := c.Logger("deployment")

	ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client, err := c.CreateClient()
	if err != nil {
		return c.Error("creating the client", err)
	}

	configuration, err := client.Internal().Bootstrap(ctx)
	if err != nil {
		return c.Error("creating the client", err)
	}

	registrationAddress := c.flagServiceIP
	if registrationAddress == "" {
		registrationAddress, err = getPublicIP()
		if err != nil {
			return c.Error("finding registration IP", err)
		}
	}

	internalDirectory, err := os.MkdirTemp("", "consul-api-gateway")
	if err != nil {
		return c.Error("creating configuration directory", err)
	}
	defer os.RemoveAll(internalDirectory)

	bootstrapFilePath := filepath.Join(internalDirectory, "bootstrap.json")

	// flush CA to disk
	caFilePath := ""
	if configuration.Consul.Ca != "" {
		caFilePath = filepath.Join(internalDirectory, "consul.ca")
		if err := os.WriteFile(caFilePath, []byte(configuration.Consul.Ca), 0600); err != nil {
			return c.Error("storing Consul CA", err)
		}
	}

	consulClient, err := consulapi.NewClient(consulFromBootstrap(configuration.Consul))
	if err != nil {
		return c.Error("initializing Consul client", err)
	}

	group, groupCtx := errgroup.WithContext(ctx)

	registry := consul.NewServiceRegistry(
		logger.Named("service-registry"),
		consulClient,
		configuration.Name,
		configuration.Namespace,
		registrationAddress,
	)
	logger.Trace("registering gateway deployment")
	if err := registry.RegisterGateway(ctx, true); err != nil {
		return c.Error("registering gateway service", err)
	}
	defer func() {
		logger.Trace("deregistering gateway service")
		// using context.Background here since the global context has
		// already been canceled at this point and we're just in a cleanup
		// function
		if err := registry.Deregister(context.Background()); err != nil {
			logger.Error("error deregistering gateway service", "error", err)
			ret = 1
		}
	}()

	envoyManager := envoy.NewManager(
		logger.Named("envoy-manager"),
		envoy.ManagerConfig{
			ID:                registry.ID(),
			Namespace:         registry.Namespace(),
			ConsulCA:          caFilePath,
			ConsulAddress:     consulBaseAddress(configuration.Consul.Server),
			ConsulXDSPort:     configuration.Consul.XdsPort,
			BootstrapFilePath: bootstrapFilePath,
			LogLevel:          c.LogLevel(),
			Token:             configuration.Consul.Token,
			EnvoyBinary:       envoyBinary,
			Output:            c.Output(),
		},
	)

	options := consul.DefaultCertManagerOptions()
	options.SDSAddress = c.flagAddress
	options.SDSPort = configuration.SdsPort
	options.Directory = internalDirectory
	certManager := consul.NewCertManager(
		logger.Named("cert-manager"),
		configuration.Consul.Server,
		*consulFromBootstrap(configuration.Consul),
		configuration.Name,
		options,
	)
	sdsConfig, err := certManager.RenderSDSConfig()
	if err != nil {
		return c.Error("rendering SDS configuration file", err)
	}

	if err := envoyManager.RenderBootstrap(sdsConfig); err != nil {
		return c.Error("rendering Envoy configuration file", err)
	}

	group.Go(func() error {
		return certManager.Manage(groupCtx)
	})

	waitCtx, waitCancel := context.WithTimeout(groupCtx, 10*time.Second)
	defer waitCancel()

	if err := certManager.WaitForWrite(waitCtx); err != nil {
		return c.Error("timeout waiting for certs to be written", err)
	}

	group.Go(func() error {
		return envoyManager.Run(groupCtx)
	})

	if err := group.Wait(); err != nil {
		return c.Error("unexpected error", err)
	}

	return c.Success("Stopping Gateway API deployment")
}

func (c *Command) CreateClient() (*api.Client, error) {
	var tlsConfig *api.TLSConfiguration
	if c.flagScheme == "https" {
		tlsConfig = &api.TLSConfiguration{
			CAFile:           c.flagCAFile,
			SkipVerification: c.flagSkipVerify,
		}
	}

	return api.CreateClient(api.ClientConfig{
		Address:          c.flagAddress,
		Port:             c.flagPort,
		GatewayToken:     c.flagToken,
		TLSConfiguration: tlsConfig,
	})
}

func (c *Command) Help() string {
	return help
}

func consulBaseAddress(address string) string {
	tokens := strings.Split(address, ":")
	if len(tokens) == 0 {
		return "localhost"
	}
	return tokens[0]
}

func consulFromBootstrap(config apiinternal.ConsulConfiguration) *consulapi.Config {
	consulCfg := consulapi.DefaultConfig()
	if config.Server != "" {
		consulCfg.Address = config.Server
	}
	if config.Token != "" {
		consulCfg.Token = config.Token
	}
	if config.Scheme != "" {
		consulCfg.Scheme = config.Scheme
	}
	if config.Ca != "" {
		consulCfg.TLSConfig.CAPem = []byte(config.Ca)
	}
	if config.ClientCertificate != "" {
		consulCfg.TLSConfig.CertPEM = []byte(config.ClientCertificate)
	}
	if config.ClientKey != "" {
		consulCfg.TLSConfig.KeyPEM = []byte(config.ClientKey)
	}
	if config.SkipVerify {
		consulCfg.TLSConfig.InsecureSkipVerify = true
	}

	return consulCfg
}

func getEnvoyBinary(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return exec.LookPath("envoy")
}

func getPublicIP() (string, error) {
	// no connection actually occurs, just lean
	// on the routing table to determine the public ip to
	// register as my service
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

const synopsis = "Run a Consul API Gateway deployment"
const help = `
Usage: consul-api-gateway deployment [options] [args]

  Runs a Gateway deployment.

  Additional flags and more advanced use cases are detailed below.
`
