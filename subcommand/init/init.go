package server

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/mitchellh/cli"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/container_init.go#L272-L306
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/connect-inject/envoy_sidecar.go#L79
// https://github.com/hashicorp/consul-k8s/blob/24be51c58461e71365ca39f113dae0379f7a1b7c/control-plane/subcommand/connect-init/command.go#L91

const (
	MetaKeyPodName = "pod-name"
	MetaKeyKubeNS  = "k8s-namespace"

	defaultBearerTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultTokenSinkFile   = "/consul/polar-inject/acl-token"

	// The number of times to attempt ACL Login.
	numLoginRetries = 3
	// The number of times to attempt to read this service (120s).
	defaultServicePollingRetries = 120
)

type Command struct {
	UI cli.Ui

	flagACLAuthMethod       string // Auth Method to use for ACLs, if enabled.
	flagPodName             string // Pod name.
	flagPodNamespace        string // Pod namespace.
	flagAuthMethodNamespace string // Consul namespace the auth-method is defined in.
	flagServiceAccountName  string // Service account name.
	flagServiceName         string // Service name.
	flagLogLevel            string
	flagLogJSON             bool
	flagBearerTokenFile     string // Location of the bearer token. Default is /var/run/secrets/kubernetes.io/serviceaccount/token.
	flagTokenSinkFile       string // Location to write the output token. Default is defaultTokenSinkFile.

	serviceRegistrationPollingAttempts uint64 // Number of times to poll for this service to be registered.

	flagSet *flag.FlagSet

	logger hclog.Logger

	once sync.Once
}

func (c *Command) init() {
	c.flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	c.flagSet.StringVar(&c.flagACLAuthMethod, "acl-auth-method", "", "Name of the auth method to login to.")
	c.flagSet.StringVar(&c.flagPodName, "pod-name", "", "Name of the pod.")
	c.flagSet.StringVar(&c.flagPodNamespace, "pod-namespace", "", "Name of the pod namespace.")
	c.flagSet.StringVar(&c.flagAuthMethodNamespace, "auth-method-namespace", "", "Consul namespace the auth-method is defined in")
	c.flagSet.StringVar(&c.flagBearerTokenFile, "bearer-token-file", defaultBearerTokenFile, "Location of the bearer token.")
	c.flagSet.StringVar(&c.flagTokenSinkFile, "token-sink-file", defaultTokenSinkFile, "Location to write the output token.")
	c.flagSet.StringVar(&c.flagServiceAccountName, "service-account-name", "", "Service account name on the pod.")
	c.flagSet.StringVar(&c.flagServiceName, "service-name", "", "Service name as specified via the pod annotation.")
	c.flagSet.StringVar(&c.flagLogLevel, "log-level", "info",
		"Log verbosity level. Supported values (in order of detail) are \"trace\", "+
			"\"debug\", \"info\", \"warn\", and \"error\".")
	c.flagSet.BoolVar(&c.flagLogJSON, "log-json", false,
		"Enable or disable JSON output format for logging.")

	if c.serviceRegistrationPollingAttempts == 0 {
		c.serviceRegistrationPollingAttempts = defaultServicePollingRetries
	}
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)

	if err := c.flagSet.Parse(args); err != nil {
		return 1
	}

	if c.flagPodName == "" {
		c.UI.Error("-pod-name must be set")
		return 1
	}
	if c.flagPodNamespace == "" {
		c.UI.Error("-pod-namespace must be set")
		return 1
	}
	if c.flagACLAuthMethod != "" && c.flagServiceAccountName == "" {
		c.UI.Error("-service-account-name must be set when ACLs are enabled")
		return 1
	}

	if c.logger == nil {
		c.logger = hclog.Default().Named("polar-init")
		c.logger.SetLevel(hclog.Trace)
	}

	cfg := api.DefaultConfig()
	consulClient, err := api.NewClient(cfg)
	if err != nil {
		c.UI.Error("An error occurred creating a Consul API client:\n\t" + err.Error())
		return 1
	}

	// First do the ACL Login, if necessary.
	if c.flagACLAuthMethod != "" {
		// loginMeta is the default metadata that we pass to the consul login API.
		loginMeta := map[string]string{"pod": fmt.Sprintf("%s/%s", c.flagPodNamespace, c.flagPodName)}
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

	// Now wait for the service to be registered. Do this by querying the Agent for a service
	// which maps to this pod+namespace.
	var proxyID string
	registrationRetryCount := 0
	var errServiceNameMismatch error
	err = backoff.Retry(func() error {
		registrationRetryCount++
		filter := fmt.Sprintf("Meta[%q] == %q and Meta[%q] == %q", MetaKeyPodName, c.flagPodName, MetaKeyKubeNS, c.flagPodNamespace)
		serviceList, err := consulClient.Agent().ServicesWithFilter(filter)
		if err != nil {
			c.logger.Error("Unable to get Agent services", "error", err)
			return err
		}
		// Wait for the service and the connect-proxy service to be registered.
		if len(serviceList) != 2 {
			c.logger.Info("Unable to find registered services; retrying")
			// Once every 10 times we're going to print this informational message to the pod logs so that
			// it is not "lost" to the user at the end of the retries when the pod enters a CrashLoop.
			if registrationRetryCount%10 == 0 {
				c.logger.Info("Check to ensure a Kubernetes service has been created for this application." +
					" If your pod is not starting also check the connect-inject deployment logs.")
			}
			return fmt.Errorf("did not find correct number of services: %d", len(serviceList))
		}
		for _, svc := range serviceList {
			c.logger.Info("Registered service has been detected", "service", svc.Service)
			if c.flagACLAuthMethod != "" {
				if c.flagServiceName != "" && c.flagServiceAccountName != c.flagServiceName {
					// Set the error but return nil so we don't retry.
					errServiceNameMismatch = fmt.Errorf("service account name %s doesn't match annotation service name %s", c.flagServiceAccountName, c.flagServiceName)
					return nil
				}

				if c.flagServiceName == "" && svc.Kind != api.ServiceKindConnectProxy && c.flagServiceAccountName != svc.Service {
					// Set the error but return nil so we don't retry.
					errServiceNameMismatch = fmt.Errorf("service account name %s doesn't match Consul service name %s", c.flagServiceAccountName, svc.Service)
					return nil
				}
			}
			if svc.Kind == api.ServiceKindConnectProxy {
				// This is the proxy service ID.
				proxyID = svc.ID
			}
		}

		if proxyID == "" {
			// In theory we can't reach this point unless we have 2 services registered against
			// this pod and neither are the connect-proxy. We don't support this case anyway, but it
			// is necessary to return from the function.
			return fmt.Errorf("unable to find registered connect-proxy service")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), c.serviceRegistrationPollingAttempts))
	if err != nil {
		c.logger.Error("Timed out waiting for service registration", "error", err)
		return 1
	}
	if errServiceNameMismatch != nil {
		c.logger.Error(errServiceNameMismatch.Error())
		return 1
	}
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
	tok, _, err := client.ACL().Login(req, &api.WriteOptions{Namespace: namespace})
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
