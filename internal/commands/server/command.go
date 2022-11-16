package server

import (
	"context"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-server-connection-manager/discovery"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
)

const (
	defaultGRPCPort      = 8502
	defaultSDSServerHost = "consul-api-gateway-controller.default.svc.cluster.local"
	defaultSDSServerPort = 9090
	// The amount of time to wait for the first cert write
	defaultCertWaitTime = 1 * time.Minute
)

type Command struct {
	UI     cli.Ui
	output io.Writer
	ctx    context.Context

	isTest bool

	flagCAFile            string // CA File for CA for Consul server
	flagCASecret          string // CA Secret for Consul server
	flagCASecretNamespace string // CA Secret namespace for Consul server

	flagConsulAddress string // Consul server address

	flagPrimaryDatacenter string // Primary datacenter, may or may not be the datacenter this controller is running in

	flagSDSServerHost string // SDS server host
	flagSDSServerPort int    // SDS server port
	flagMetricsPort   int    // Port for prometheus metrics
	flagPprofPort     int    // Port for pprof profiling
	flagK8sContext    string // context to use
	flagK8sNamespace  string // namespace we're run in

	// Consul namespaces
	flagConsulDestinationNamespace string
	flagMirrorK8SNamespaces        bool
	flagMirrorK8SNamespacePrefix   string

	// Logging
	flagLogLevel string
	flagLogJSON  bool

	flagSet *flag.FlagSet
	once    sync.Once
}

// New returns a new server command
func New(ctx context.Context, ui cli.Ui, logOutput io.Writer) *Command {
	return &Command{UI: ui, output: logOutput, ctx: ctx}
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	c.flagSet.StringVar(&c.flagCAFile, "ca-file", "", "Path to CA for Consul server.")
	c.flagSet.StringVar(&c.flagCASecret, "ca-secret", "", "CA Secret for Consul server.")
	c.flagSet.StringVar(&c.flagCASecretNamespace, "ca-secret-namespace", "default", "CA Secret namespace for Consul server.")
	c.flagSet.StringVar(&c.flagConsulAddress, "consul-address", "", "Consul Address.")
	c.flagSet.StringVar(&c.flagPrimaryDatacenter, "primary-datacenter", "", "Name of the primary Consul datacenter")
	c.flagSet.StringVar(&c.flagSDSServerHost, "sds-server-host", defaultSDSServerHost, "SDS Server Host.")
	c.flagSet.StringVar(&c.flagK8sContext, "k8s-context", "", "Kubernetes context to use.")
	c.flagSet.StringVar(&c.flagK8sNamespace, "k8s-namespace", "", "Kubernetes namespace to use.")
	c.flagSet.IntVar(&c.flagSDSServerPort, "sds-server-port", defaultSDSServerPort, "SDS Server Port.")
	c.flagSet.IntVar(&c.flagMetricsPort, "metrics-port", 0, "Metrics port, if not set, metrics are not enabled.")
	c.flagSet.IntVar(&c.flagPprofPort, "pprof-port", 0, "Go pprof port, if not set, profiling is not enabled.")

	{
		// Consul namespaces
		c.flagSet.StringVar(&c.flagConsulDestinationNamespace, "consul-destination-namespace", "", "Consul namespace to register gateway services.")
		c.flagSet.BoolVar(&c.flagMirrorK8SNamespaces, "mirroring-k8s", false, "Register Consul gateway services based on Kubernetes namespace.")
		c.flagSet.StringVar(&c.flagMirrorK8SNamespacePrefix, "mirroring-k8s-prefix", "", "Namespace prefix for Consul services when mirroring Kubernetes namespaces.")
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

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)
	c.flagSet.SetOutput(c.output)

	if err := c.flagSet.Parse(args); err != nil {
		return 1
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Level:           hclog.LevelFromString(c.flagLogLevel),
		Output:          c.output,
		JSONFormat:      c.flagLogJSON,
		IncludeLocation: true,
	}).Named("consul-api-gateway-server")

	cfg := k8s.Defaults()
	restConfig, err := config.GetConfigWithContext(c.flagK8sContext)
	if err != nil {
		logger.Error("error getting kubernetes configuration", "error", err)
		return 1
	}
	cfg.RestConfig = restConfig
	cfg.SDSServerHost = c.flagSDSServerHost
	cfg.SDSServerPort = c.flagSDSServerPort
	cfg.Namespace = c.flagK8sNamespace
	cfg.PrimaryDatacenter = c.flagPrimaryDatacenter

	consulCfg := api.DefaultConfig()
	if c.flagCAFile != "" {
		consulCfg.TLSConfig.CAFile = c.flagCAFile
	}

	if c.flagCASecret != "" {
		// if we're pulling the cert from a secret, then we override the location
		// where we store it
		file, err := ioutil.TempFile("", "consul-api-gateway")
		if err != nil {
			logger.Error("error creating the kubernetes controller", "error", err)
			return 1
		}
		defer os.Remove(file.Name())
		consulCfg.TLSConfig.CAFile = file.Name()

		if err := utils.WriteSecretCertFile(restConfig, c.flagCASecret, file.Name(), c.flagCASecretNamespace); err != nil {
			logger.Error("error creating the kubernetes controller", "error", err)
			return 1
		}
	}
	// CA file can be set by cli flag or 'CONSUL_CACERT' env var
	if consulCfg.TLSConfig.CAFile != "" {
		consulCfg.Scheme = "https"
		consulCA, err := ioutil.ReadFile(consulCfg.TLSConfig.CAFile)
		if err != nil {
			logger.Error("error creating the kubernetes controller", "error", err)
			return 1
		}

		cfg.CACert = string(consulCA)
	}

	if c.flagConsulAddress != "" {
		consulCfg.Address = c.flagConsulAddress
	}

	cfg.ConsulNamespaceConfig = k8s.ConsulNamespaceConfig{
		ConsulDestinationNamespace:      c.flagConsulDestinationNamespace,
		MirrorKubernetesNamespaces:      c.flagMirrorK8SNamespaces,
		MirrorKubernetesNamespacePrefix: c.flagMirrorK8SNamespacePrefix,
	}

	consulHTTPAddressOrCommand, port, err := parseConsulHTTPAddress()
	if err != nil {
		logger.Error("error reading "+consulHTTPAddressEnvName, "error", err)
		return 1
	}

	tlsCfg, err := api.SetupTLSConfig(&consulCfg.TLSConfig)
	if err != nil {
		logger.Error("could not set up tls config", err)
		return 1
	}

	var token string
	if consulCfg.TokenFile != "" {
		data, err := ioutil.ReadFile(consulCfg.TokenFile)
		if err != nil {
			logger.Error("error loading token file", err)
			return 1
		}

		if t := strings.TrimSpace(string(data)); t != "" {
			token = t
		}
	}
	if consulCfg.Token != "" {
		token = consulCfg.Token
	}

	consulClientConfig := consul.ClientConfig{
		ApiClientConfig: consulCfg,
		Addresses:       consulHTTPAddressOrCommand,
		HTTPPort:        port,
		GRPCPort:        grpcPort(),
		PlainText:       consulCfg.Scheme == "http",
		TLS:             tlsCfg,
		Credentials: discovery.Credentials{
			Type: discovery.CredentialsTypeStatic,
			Static: discovery.StaticTokenCredential{
				Token: token,
			},
		},
		Logger: logger,
	}

	return RunServer(ServerConfig{
		Context:            context.Background(),
		Logger:             logger,
		ConsulConfig:       consulCfg,
		K8sConfig:          cfg,
		ProfilingPort:      c.flagPprofPort,
		MetricsPort:        c.flagMetricsPort,
		PrimaryDatacenter:  c.flagPrimaryDatacenter,
		isTest:             c.isTest,
		ConsulClientConfig: consulClientConfig,
	})
}

func grpcPort() int {
	port := os.Getenv("CONSUL_GRPC_PORT")
	p, err := strconv.Atoi(port)
	if err == nil {
		return p
	}
	return defaultGRPCPort
}

func (c *Command) Synopsis() string {
	return "Starts the consul-api-gateway control plane server"
}

func (c *Command) Help() string {
	return `
Usage: consul-api-gateway server [options]
`
}
