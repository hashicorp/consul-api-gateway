package consul

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"text/template"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

const (
	RootCAFile           = "root-ca.pem"
	ClientCertFile       = "client.crt"
	ClientPrivateKeyFile = "client.pem"
	SDSCertConfigFile    = "tls-sds.json"
	SDSCAConfigFile      = "validation-context-sds.json"

	defaultCertificateDirectory = "/certs"
	defaultSignalOnNWrites      = 1
	defaultSDSAddress           = "localhost"
	defaultSDSPort              = 9090
)

var (
	sdsClusterTemplate    = template.New("sdsCluster")
	sdsCertConfigTemplate = template.New("sdsCert")
	sdsCAConfigTemplate   = template.New("sdsCA")
)

type sdsClusterArgs struct {
	Name              string
	CertSDSConfigPath string
	CASDSConfigPath   string
	AddressType       string
	SDSAddress        string
	SDSPort           int
}

type sdsCertConfigArgs struct {
	CertPath           string
	CertPrivateKeyPath string
}

type sdsCAConfigArgs struct {
	CAPath string
}

func init() {
	_, err := sdsClusterTemplate.Parse(sdsClusterJSONTemplate)
	if err != nil {
		panic(err)
	}
	_, err = sdsCertConfigTemplate.Parse(sdsCertConfigJSONTemplate)
	if err != nil {
		panic(err)
	}
	_, err = sdsCAConfigTemplate.Parse(sdsCAConfigJSONTemplate)
	if err != nil {
		panic(err)
	}
}

// CertManagerOptions contains the optional configuration used to initialize a CertManager.
type CertManagerOptions struct {
	Directory       string
	SignalOnNWrites int
	Tries           uint64
	SDSAddress      string
	SDSPort         int
}

// DefaultCertManagerOptions returns the default options for a CertManager instance.
func DefaultCertManagerOptions() *CertManagerOptions {
	return &CertManagerOptions{
		Directory:       defaultCertificateDirectory,
		SignalOnNWrites: defaultSignalOnNWrites,
		Tries:           defaultMaxAttempts,
		SDSAddress:      defaultSDSAddress,
		SDSPort:         defaultSDSPort,
	}
}

// CertManager handles Consul leaf certificate management and certificate rotation.
// Once a leaf certificate has expired, it generates a new certificate and writes
// it to the location given in the configuration options with which it was created.
type CertManager struct {
	consul *api.Client
	logger hclog.Logger

	service    string
	directory  string
	sdsAddress string
	sdsPort    int

	signalWrites int
	writesLeft   int
	writes       chan struct{}

	tries           uint64
	backoffInterval time.Duration

	lock sync.RWMutex
}

// NewCertManager creates a new CertManager instance.
func NewCertManager(logger hclog.Logger, consul *api.Client, service string, options *CertManagerOptions) *CertManager {
	if options == nil {
		options = DefaultCertManagerOptions()
	}
	return &CertManager{
		consul:          consul,
		logger:          logger,
		sdsAddress:      options.SDSAddress,
		sdsPort:         options.SDSPort,
		service:         service,
		directory:       options.Directory,
		signalWrites:    options.SignalOnNWrites,
		writesLeft:      options.SignalOnNWrites,
		writes:          make(chan struct{}, options.SignalOnNWrites),
		tries:           options.Tries,
		backoffInterval: defaultBackoffInterval,
	}
}

// Manage is the main run loop of the manager and should be run in a go routine.
// It should be passed a cancellable context that signals when the manager should
// stop and return. If it receives an unexpected error the loop exits.
func (c *CertManager) Manage(ctx context.Context) error {
	for {
		var root *api.CARoot
		var clientCert *api.LeafCert
		var err error

		err = backoff.Retry(func() error {
			root, clientCert, err = c.getCerts(ctx)
			if err != nil {
				c.logger.Error("error requesting certificates", "error", err)
			}
			return err
		}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(c.backoffInterval), c.tries), ctx))
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// we intentionally canceled the context, just return
				return nil
			}
			return err
		}

		err = c.persist(root, clientCert)
		if err != nil {
			return err
		}

		expiresIn := time.Until(clientCert.ValidBefore)
		select {
		case <-time.After(expiresIn):
			// loop
		case <-ctx.Done():
			return nil
		}
	}
}

func (c *CertManager) getCerts(ctx context.Context) (*api.CARoot, *api.LeafCert, error) {
	options := &api.QueryOptions{}
	clientCert, _, err := c.consul.Agent().ConnectCALeaf(c.service, options.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("error generating client leaf certificate: %w", err)
	}
	roots, _, err := c.consul.Agent().ConnectCARoots(options.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("error retrieving root CA: %w", err)
	}
	for _, root := range roots.Roots {
		if root.Active {
			return root, clientCert, nil
		}
	}
	return nil, nil, errors.New("root CA not found")
}

func (c *CertManager) persist(root *api.CARoot, client *api.LeafCert) error {
	rootCAFile := path.Join(c.directory, RootCAFile)
	clientCertFile := path.Join(c.directory, ClientCertFile)
	clientPrivateKeyFile := path.Join(c.directory, ClientPrivateKeyFile)

	c.lock.Lock()
	defer c.lock.Unlock()

	if err := os.WriteFile(rootCAFile, []byte(root.RootCertPEM), 0600); err != nil {
		return fmt.Errorf("error writing root CA fiile: %w", err)
	}
	if err := os.WriteFile(clientCertFile, []byte(client.CertPEM), 0600); err != nil {
		return fmt.Errorf("error writing client cert fiile: %w", err)
	}
	if err := os.WriteFile(clientPrivateKeyFile, []byte(client.PrivateKeyPEM), 0600); err != nil {
		return fmt.Errorf("error writing client private key fiile: %w", err)
	}

	if c.writesLeft > 0 {
		c.writes <- struct{}{}
		c.writesLeft--
	}

	return nil
}

// RootCA returns the current CA cert
func (c *CertManager) RootCA() ([]byte, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return os.ReadFile(path.Join(c.directory, RootCAFile))
}

// Certificate returns the current leaf cert
func (c *CertManager) Certificate() ([]byte, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return os.ReadFile(path.Join(c.directory, ClientCertFile))
}

// PrivateKey returns the current leaf cert private key
func (c *CertManager) PrivateKey() ([]byte, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return os.ReadFile(path.Join(c.directory, ClientPrivateKeyFile))
}

// Wait acts as a signalling mechanism for when the certificates are
// written to disk. It is intended to be used for use-cases where initial certificates
// must be in place prior to being referenced by a consumer.
func (c *CertManager) Wait(ctx context.Context) error {
	for {
		if c.signalWrites <= 0 {
			break
		}
		select {
		case <-c.writes:
			c.signalWrites--
		case <-ctx.Done():
			return errors.New("wait canceled")
		}
	}
	return nil
}

func (c *CertManager) RenderSDSConfig() (string, error) {
	var (
		sdsCertConfig bytes.Buffer
		sdsCAConfig   bytes.Buffer
		sdsConfig     bytes.Buffer
	)

	rootCAFile := path.Join(c.directory, RootCAFile)
	clientCertFile := path.Join(c.directory, ClientCertFile)
	clientPrivateKeyFile := path.Join(c.directory, ClientPrivateKeyFile)

	sdsCertConfigPath := path.Join(c.directory, SDSCertConfigFile)
	sdsCAConfigPath := path.Join(c.directory, SDSCAConfigFile)

	// write the cert and ca configs to disk first

	if err := sdsCertConfigTemplate.Execute(&sdsCertConfig, &sdsCertConfigArgs{
		CertPath:           clientCertFile,
		CertPrivateKeyPath: clientPrivateKeyFile,
	}); err != nil {
		return "", err
	}
	if err := sdsCAConfigTemplate.Execute(&sdsCAConfig, &sdsCAConfigArgs{
		CAPath: rootCAFile,
	}); err != nil {
		return "", err
	}
	if err := os.WriteFile(sdsCertConfigPath, sdsCertConfig.Bytes(), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(sdsCAConfigPath, sdsCAConfig.Bytes(), 0600); err != nil {
		return "", err
	}

	// now render out the json for the sds config itself and return it

	if err := sdsClusterTemplate.Execute(&sdsConfig, &sdsClusterArgs{
		Name:              "sds-cluster",
		CertSDSConfigPath: sdsCertConfigPath,
		CASDSConfigPath:   sdsCAConfigPath,
		AddressType:       addressTypeForAddress(c.sdsAddress),
		SDSAddress:        c.sdsAddress,
		SDSPort:           c.sdsPort,
	}); err != nil {
		return "", err
	}

	return sdsConfig.String(), nil
}

const sdsClusterJSONTemplate = `
{
  "name":"{{ .Name }}",
  "connect_timeout":"5s",
  "type":"{{ .AddressType }}",
  "transport_socket":{
     "name":"tls",
     "typed_config":{
        "@type":"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
        "common_tls_context":{
           "tls_certificate_sds_secret_configs":[
              {
                 "name":"tls_sds",
                 "sds_config":{
                    "path":"{{ .CertSDSConfigPath }}"
                 }
              }
           ],
           "validation_context_sds_secret_config":{
              "name":"validation_context_sds",
              "sds_config":{
                 "path":"{{ .CASDSConfigPath }}"
              }
           }
        }
     }
  },
  "http2_protocol_options":{},
  "loadAssignment":{
     "clusterName":"{{ .Name }}",
     "endpoints":[
        {
           "lbEndpoints":[
              {
                 "endpoint":{
                    "address":{
                       "socket_address":{
                          "address":"{{ .SDSAddress }}",
                          "port_value":{{ .SDSPort }}
                       }
                    }
                 }
              }
           ]
        }
     ]
  }
}
`

const sdsCertConfigJSONTemplate = `
{
   "resources": [
     {
       "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret",
       "name": "tls_sds",
       "tls_certificate": {
         "certificate_chain": {
           "filename": "{{ .CertPath }}"
         },
         "private_key": {
           "filename": "{{ .CertPrivateKeyPath }}"
         }
       }
     }
   ]
 }
 `

const sdsCAConfigJSONTemplate = `
 {
   "resources": [
     {
       "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret",
       "name": "validation_context_sds",
       "validation_context": {
         "trusted_ca": {
           "filename": "{{ .CAPath }}"
         }
       }
     }
   ]
 }
 `
