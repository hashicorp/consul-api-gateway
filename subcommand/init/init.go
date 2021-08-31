package server

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/uuid"
	"github.com/mitchellh/cli"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/container_init.go#L272-L306
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/envoy_sidecar.go#L79
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/subcommand/connect-init/command.go#L91
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/endpoints_controller.go#L403

const (
	MetaKeyPodName = "pod-name"
	MetaKeyKubeNS  = "k8s-namespace"

	defaultBearerTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultTokenSinkFile   = "/polar/acl-token"
	defaultServiceIDFile   = "/polar/id"

	// The number of times to attempt ACL Login.
	numLoginRetries = 3
	// The number of times to attempt to read this service (120s).
	defaultServiceRegistrationRetries = 120
)

type Command struct {
	UI cli.Ui

	// Consul params
	flagConsulHTTPAddress string // Address for Consul HTTP API.
	flagConsulCACertFile  string // Root CA file for Consul

	// Gateway params
	flagGatewayID        string // Gateway iD.
	flagGatewayIP        string // Gateway ip.
	flagGatewayPort      int    // Gateway port.
	flagGatewayName      string // Gateway name.
	flagGatewayNamespace string // Gateway namespace.

	// Auth
	flagACLAuthMethod       string // Auth Method to use for ACLs, if enabled.
	flagAuthMethodNamespace string // Consul namespace the auth-method is defined in.
	flagBearerTokenFile     string // Location of the bearer token. Default is /var/run/secrets/kubernetes.io/serviceaccount/token.
	flagTokenSinkFile       string // Location to write the output token. Default is defaultTokenSinkFile.

	// Logging
	flagLogLevel string
	flagLogJSON  bool

	// Deregistration
	flagDeregister bool

	serviceRegistrationAttempts uint64 // Number of times to poll for this service to be registered.

	flagSet *flag.FlagSet

	logger hclog.Logger

	once sync.Once
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	c.flagSet.BoolVar(&c.flagDeregister, "deregister", false, "Deregister instead of bootstrapping.")
	c.flagSet.StringVar(&c.flagConsulHTTPAddress, "consul-http-address", "", "Address of Consul.")
	c.flagSet.StringVar(&c.flagConsulCACertFile, "consul-ca-cert-file", "", "CA Root file for Consul.")
	c.flagSet.StringVar(&c.flagGatewayID, "gateway-id", "", "ID of the gateway.")
	c.flagSet.StringVar(&c.flagGatewayIP, "gateway-ip", "", "IP of the gateway.")
	c.flagSet.IntVar(&c.flagGatewayPort, "gateway-port", 0, "Port of the gateway.")
	c.flagSet.StringVar(&c.flagGatewayName, "gateway-name", "", "Name of the gateway.")
	c.flagSet.StringVar(&c.flagGatewayNamespace, "gateway-namespace", "default", "Name of the gateway namespace.")
	c.flagSet.StringVar(&c.flagACLAuthMethod, "acl-auth-method", "", "Name of the auth method to login with.")
	c.flagSet.StringVar(&c.flagAuthMethodNamespace, "auth-method-namespace", "default", "Consul namespace the auth-method is defined in")
	c.flagSet.StringVar(&c.flagBearerTokenFile, "bearer-token-file", defaultBearerTokenFile, "Location of the bearer token.")
	c.flagSet.StringVar(&c.flagTokenSinkFile, "token-sink-file", defaultTokenSinkFile, "Location to write the output token.")
	c.flagSet.StringVar(&c.flagLogLevel, "log-level", "info",
		"Log verbosity level. Supported values (in order of detail) are \"trace\", "+
			"\"debug\", \"info\", \"warn\", and \"error\".")
	c.flagSet.BoolVar(&c.flagLogJSON, "log-json", false,
		"Enable or disable JSON output format for logging.")

	if c.serviceRegistrationAttempts == 0 {
		c.serviceRegistrationAttempts = defaultServiceRegistrationRetries
	}
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)

	if err := c.flagSet.Parse(args); err != nil {
		return 1
	}

	if c.flagConsulHTTPAddress == "" {
		c.UI.Error("-consul-http-address must be set")
		return 1
	}
	if c.flagGatewayIP == "" {
		c.UI.Error("-gateway-ip must be set")
		return 1
	}
	if c.flagGatewayPort == 0 {
		c.UI.Error("-gateway-port must be set")
		return 1
	}
	if c.flagGatewayName == "" {
		c.UI.Error("-gateway-name must be set")
		return 1
	}

	if c.logger == nil {
		c.logger = hclog.Default().Named("polar-init")
		c.logger.SetLevel(hclog.Trace)
	}

	if c.flagGatewayID == "" {
		c.flagGatewayID = uuid.New().String()
	}

	cfg := api.DefaultConfig()
	cfg.Address = c.flagConsulHTTPAddress
	if c.flagConsulCACertFile != "" {
		cfg.Scheme = "https"
		cfg.TLSConfig.CAFile = c.flagConsulCACertFile
	}
	consulClient, err := api.NewClient(cfg)
	if err != nil {
		c.UI.Error("An error occurred creating a Consul API client:\n\t" + err.Error())
		return 1
	}

	// First do the ACL Login, if necessary.
	if c.flagACLAuthMethod != "" {
		// loginMeta is the default metadata that we pass to the consul login API.
		loginMeta := map[string]string{"polar": fmt.Sprintf("%s/%s", c.flagGatewayNamespace, c.flagGatewayName)}
		err = backoff.Retry(func() error {
			err := ConsulLogin(consulClient, c.flagBearerTokenFile, c.flagACLAuthMethod, c.flagTokenSinkFile, c.flagAuthMethodNamespace, loginMeta)
			if err != nil {
				c.logger.Error("Consul login failed; retrying", "error", err)
			}
			return err
		}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), numLoginRetries))
		if err != nil {
			c.logger.Error("Hit maximum retries for consul login", "error", err)
			return 1
		}
		// Now update the client so that it will read the ACL token we just fetched.
		cfg.TokenFile = c.flagTokenSinkFile
		consulClient, err = api.NewClient(cfg)
		if err != nil {
			c.logger.Error("Unable to update client connection", "error", err)
			return 1
		}
		c.logger.Info("Consul login complete")
	}

	if c.flagDeregister {
		return c.deregister(consulClient)
	}
	// Envoy will watch these references and reload the tls context when changed. When the kubernetes deployment is created, a config map for these files will be created and mounted in the expected locations. The certificates and keys they reference will also be created as kubernetes secrets and mounted to their expected locations on the pod.
	// When the leaf certificate needs to be rotated, the polar controller will update the kubernetes secret which will update the file on disk in the pod, triggering envoy to reload the tls context with the updated credentials.
	// http://127.0.0.1:8500/v1/agent/connect/ca/leaf/web
	return c.register(consulClient)
}

func (c *Command) deregister(consulClient *api.Client) int {
	data, err := os.ReadFile(defaultServiceIDFile)
	if err != nil {
		c.logger.Error("Unable to read service ID file", "error", err)
		return 1
	}

	serviceID := strings.TrimSpace(string(data))

	// Now deregister the envoy service in Consul.
	err = backoff.Retry(func() error {
		err := consulClient.Agent().ServiceDeregister(serviceID)
		if err != nil {
			c.logger.Error("failed to deregister gateway service", "error", err)
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), c.serviceRegistrationAttempts))
	if err != nil {
		c.logger.Error("Timed out waiting for service deregistration", "error", err)
		return 1
	}

	c.logger.Info("Service deregistration complete")
	return 0
}

func (c *Command) register(consulClient *api.Client) int {
	// Now register the envoy service in Consul.
	err := backoff.Retry(func() error {
		registration := &api.AgentServiceRegistration{
			ID:      c.flagGatewayID,
			Name:    c.flagGatewayName,
			Port:    c.flagGatewayPort,
			Address: c.flagGatewayIP,
			Checks: api.AgentServiceChecks{
				{
					Name:                           "Gateway Listener",
					TCP:                            fmt.Sprintf("%s:%d", c.flagGatewayIP, c.flagGatewayPort),
					Interval:                       "10s",
					DeregisterCriticalServiceAfter: "1m",
				},
			},
		}
		if c.flagGatewayNamespace != "default" {
			registration.Namespace = "default"
		}
		err := consulClient.Agent().ServiceRegister(registration)
		if err != nil {
			c.logger.Error("failed to register gateway service", "name", registration.Name, "error", err)
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), c.serviceRegistrationAttempts))
	if err != nil {
		c.logger.Error("Timed out waiting for service registration", "error", err)
		return 1
	}

	if err := WriteFileWithPerms(defaultServiceIDFile, c.flagGatewayID, 0444); err != nil {
		c.logger.Error("failed to write service id file", "error", err)
		return 1
	}

	c.logger.Info("Service registration complete")
	return 0
}

func (c *Command) Synopsis() string {
	return "Inject polar init command"
}

func (c *Command) Help() string {
	return `
Usage: polar init [options]

	Bootstraps polar pod components.
	Not intended for stand-alone use.
`
}

// ConsulLogin issues an ACL().Login to Consul and writes out the token to tokenSinkFile.
// The logic of this is taken from the `consul login` command.
func ConsulLogin(client *api.Client, bearerTokenFile, authMethodName, tokenSinkFile, namespace string, meta map[string]string) error {
	if meta == nil {
		return fmt.Errorf("invalid meta")
	}
	data, err := os.ReadFile(bearerTokenFile)
	if err != nil {
		return fmt.Errorf("unable to read bearerTokenFile: %v, err: %v", bearerTokenFile, err)
	}
	bearerToken := strings.TrimSpace(string(data))
	if bearerToken == "" {
		return fmt.Errorf("no bearer token found in %s", bearerTokenFile)
	}
	// Do the login.
	req := &api.ACLLoginParams{
		AuthMethod:  authMethodName,
		BearerToken: bearerToken,
		Meta:        meta,
	}
	opts := &api.WriteOptions{}
	if namespace != "default" {
		opts.Namespace = namespace
	}
	tok, _, err := client.ACL().Login(req, opts)
	if err != nil {
		return fmt.Errorf("error logging in: %s", err)
	}

	if err := WriteFileWithPerms(tokenSinkFile, tok.SecretID, 0444); err != nil {
		return fmt.Errorf("error writing token to file sink: %v", err)
	}
	return nil
}

// WriteFileWithPerms will write payload as the contents of the outputFile and set permissions after writing the contents. This function is necessary since using ioutil.WriteFile() alone will create the new file with the requested permissions prior to actually writing the file, so you can't set read-only permissions.
func WriteFileWithPerms(outputFile, payload string, mode os.FileMode) error {
	// os.WriteFile truncates existing files and overwrites them, but only if they are writable.
	// If the file exists it will already likely be read-only. Remove it first.
	if _, err := os.Stat(outputFile); err == nil {
		if err = os.Remove(outputFile); err != nil {
			return fmt.Errorf("unable to delete existing file: %s", err)
		}
	}
	if err := os.WriteFile(outputFile, []byte(payload), os.ModePerm); err != nil {
		return fmt.Errorf("unable to write file: %s", err)
	}
	return os.Chmod(outputFile, mode)
}
