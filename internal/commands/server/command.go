package server

import (
	"context"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/mitchellh/cli"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s"
)

const (
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
	flagConsulAddress     string // Consul server address
	flagSDSServerHost     string // SDS server host
	flagSDSServerPort     int    // SDS server port
	flagMetricsPort       int    // Port for prometheus metrics
	flagPprofPort         int    // Port for pprof profiling
	flagK8sContext        string // context to use
	flagK8sNamespace      string // namespace we're run in

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
	c.flagSet.StringVar(&c.flagCASecretNamespace, "ca-secret-namespace", "", "CA Secret namespace for Consul server.")
	c.flagSet.StringVar(&c.flagConsulAddress, "consul-address", "", "Consul Address.")
	c.flagSet.StringVar(&c.flagSDSServerHost, "sds-server-host", defaultSDSServerHost, "SDS Server Host.")
	c.flagSet.StringVar(&c.flagK8sContext, "k8s-context", "", "Kubernetes context to use.")
	c.flagSet.StringVar(&c.flagK8sNamespace, "k8s-namespace", "", "Kubernetes namespace to use.")
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

	consulCfg := api.DefaultConfig()
	cfg := k8s.Defaults()

	if c.flagCAFile != "" {
		consulCfg.TLSConfig.CAFile = c.flagCAFile
		cfg.CACertFile = c.flagCAFile
		consulCfg.Scheme = "https"
	}

	if c.flagCASecret != "" {
		cfg.CACertSecret = c.flagCASecret
		if c.flagCASecretNamespace != "" {
			cfg.CACertSecretNamespace = c.flagCASecretNamespace
		}

		// if we're pulling the cert from a secret, then we override the location
		// where we store it
		file, err := ioutil.TempFile("", "consul-api-gateway")
		if err != nil {
			logger.Error("error creating the kubernetes controller", "error", err)
			return 1
		}
		defer os.Remove(file.Name())
		cfg.CACertFile = file.Name()
		consulCfg.TLSConfig.CAFile = file.Name()
		consulCfg.Scheme = "https"
	}

	if c.flagConsulAddress != "" {
		consulCfg.Address = c.flagConsulAddress
	}

	restConfig, err := config.GetConfigWithContext(c.flagK8sContext)
	if err != nil {
		logger.Error("error getting kubernetes configuration", "error", err)
		return 1
	}
	cfg.RestConfig = restConfig
	cfg.SDSServerHost = c.flagSDSServerHost
	cfg.SDSServerPort = c.flagSDSServerPort
	cfg.Namespace = c.flagK8sNamespace

	return RunServer(ServerConfig{
		Context:       context.Background(),
		Logger:        logger,
		ConsulConfig:  consulCfg,
		K8sConfig:     cfg,
		ProfilingPort: c.flagPprofPort,
		MetricsPort:   c.flagMetricsPort,
		isTest:        c.isTest,
	})
}

func (c *Command) Synopsis() string {
	return "Starts the consul-api-gateway control plane server"
}

func (c *Command) Help() string {
	return `
Usage: consul-api-gateway server [options]
`
}
