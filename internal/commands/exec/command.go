package exec

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/cli"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
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
	UI     cli.Ui
	output io.Writer
	ctx    context.Context

	isTest bool

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

	once sync.Once
}

// New returns a new exec command
func New(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	// we synchronize writes here because we have thread-safe
	// logger instance and a spawned command sharing the same
	// writer across go-routines
	output := common.SynchronizeWriter(logOutput)
	return &Command{UI: ui, output: output, ctx: ctx}
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
		c.flagSet.StringVar(&c.flagGatewayNamespace, "gateway-namespace", "", "Name of the gateway namespace.")
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
	c.flagSet.SetOutput(c.output)

	if err := c.flagSet.Parse(args); err != nil {
		c.UI.Error("There was an error parsing the command line flags:\n\t" + err.Error())
		return 1
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Level:           hclog.LevelFromString(c.flagLogLevel),
		Output:          c.output,
		JSONFormat:      c.flagLogJSON,
		IncludeLocation: true,
	}).Named("consul-api-gateway-exec")

	if err := c.validateFlags(); err != nil {
		logger.Error("invalid flags", "error", err)
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
		logger.Error("error creating consul client", "error", err)
		return 1
	}

	var bearerToken string
	if c.flagACLAuthMethod != "" {
		data, err := os.ReadFile(c.flagBearerTokenFile)
		if err != nil {
			logger.Error("error reading bearer token", "error", err)
			return 1
		}
		bearerToken = strings.TrimSpace(string(data))
	}

	return RunExec(ExecConfig{
		Context:      c.ctx,
		Logger:       logger,
		LogLevel:     c.flagLogLevel,
		ConsulClient: consulClient,
		ConsulConfig: *cfg,
		AuthConfig: AuthConfig{
			Method:    c.flagACLAuthMethod,
			Namespace: c.flagAuthMethodNamespace,
			Token:     bearerToken,
		},
		GatewayConfig: GatewayConfig{
			Host:      c.flagGatewayHost,
			Name:      c.flagGatewayName,
			Namespace: c.flagGatewayNamespace,
		},
		EnvoyConfig: EnvoyConfig{
			CACertificateFile: c.flagConsulCACertFile,
			XDSAddress:        c.flagConsulHTTPAddress,
			XDSPort:           c.flagConsulXDSPort,
			SDSAddress:        c.flagSDSServerAddress,
			SDSPort:           c.flagSDSServerPort,
			BootstrapFile:     c.flagBootstrapPath,
			Binary:            "envoy",
			Output:            c.output,
		},
		isTest: c.isTest,
	})
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

func (c *Command) Synopsis() string {
	return "consul-api-gateway exec command"
}

func (c *Command) Help() string {
	return `
Usage: consul-api-gateway exec [options]

	Handles service registration, certificate rotation, and spawning envoy.
`
}
