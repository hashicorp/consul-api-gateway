package consul

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"sync"
	"text/template"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
)

const (
	RootCAFile           = "root-ca.pem"
	ClientCertFile       = "client.crt"
	ClientPrivateKeyFile = "client.pem"
	SDSCertConfigFile    = "tls-sds.json"
	SDSCAConfigFile      = "validation-context-sds.json"

	defaultSDSAddress = "localhost"
	defaultSDSPort    = 9090
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
	Directory  string
	SDSAddress string
	SDSPort    int
}

// DefaultCertManagerOptions returns the default options for a CertManager instance.
func DefaultCertManagerOptions() *CertManagerOptions {
	return &CertManagerOptions{
		SDSAddress: defaultSDSAddress,
		SDSPort:    defaultSDSPort,
	}
}

// certWriter acts as the function used for persisting certificates
// for a CertManager instance
type certWriter func() error

// CertManager handles Consul leaf certificate management and certificate rotation.
// Once a leaf certificate has expired, it generates a new certificate and writes
// it to the location given in the configuration options with which it was created.
type CertManager struct {
	consul *api.Client
	logger hclog.Logger

	service         string
	directory       string
	configDirectory string // only used for testing
	sdsAddress      string
	sdsPort         int

	lock sync.RWMutex

	signalled        bool
	initializeSignal chan struct{}

	// cached values
	ca                  []byte
	certificate         []byte
	privateKey          []byte
	tlsCertificate      *tls.Certificate
	rootCertificatePool *x509.CertPool
	spiffeURL           *url.URL

	// watches
	rootWatch *watch.Plan
	leafWatch *watch.Plan

	// these can be overwritten to modify retry logic in testing
	writeCerts     certWriter
	skipExtraFetch bool
}

// NewCertManager creates a new CertManager instance.
func NewCertManager(logger hclog.Logger, consul *api.Client, service string, options *CertManagerOptions) *CertManager {
	if options == nil {
		options = DefaultCertManagerOptions()
	}
	manager := &CertManager{
		consul:           consul,
		logger:           logger,
		sdsAddress:       options.SDSAddress,
		sdsPort:          options.SDSPort,
		service:          service,
		configDirectory:  options.Directory,
		directory:        options.Directory,
		initializeSignal: make(chan struct{}),
	}
	manager.writeCerts = manager.persist
	return manager
}

func (c *CertManager) handleRootWatch(blockParam watch.BlockingParamVal, raw interface{}) {
	if raw == nil {
		c.logger.Error("received nil interface")
		return
	}
	v, ok := raw.(*api.CARootList)
	if !ok || v == nil {
		c.logger.Error("got invalid response from root watch")
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	roots := x509.NewCertPool()
	for _, root := range v.Roots {
		roots.AppendCertsFromPEM([]byte(root.RootCertPEM))
		if root.Active {
			block, _ := pem.Decode([]byte(root.RootCertPEM))
			caCert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				c.logger.Error("failed to parse root certificate")
				return
			}
			for _, uri := range caCert.URIs {
				if uri.Scheme == "spiffe" {
					c.spiffeURL = uri
					break
				}
			}
			c.ca = []byte(root.RootCertPEM)
		}
	}

	c.rootCertificatePool = roots

	if err := c.writeCerts(); err != nil {
		c.logger.Error("error persisting root certificates")
		return
	}
}

func (c *CertManager) handleLeafWatch(blockParam watch.BlockingParamVal, raw interface{}) {
	if raw == nil {
		c.logger.Error("received nil interface")
		return // ignore
	}
	v, ok := raw.(*api.LeafCert)
	if !ok || v == nil {
		c.logger.Error("got invalid response from leaf watch")
		return
	}

	// Got new leaf, update the tls.Configs
	cert, err := tls.X509KeyPair([]byte(v.CertPEM), []byte(v.PrivateKeyPEM))
	if err != nil {
		c.logger.Error("failed to parse new leaf cert", "error", err)
		return
	}

	metrics.Registry.IncrCounter(metrics.ConsulLeafCertificateFetches, 1)

	c.lock.Lock()
	defer c.lock.Unlock()

	c.certificate = []byte(v.CertPEM)
	c.privateKey = []byte(v.PrivateKeyPEM)
	c.tlsCertificate = &cert

	if err := c.writeCerts(); err != nil {
		c.logger.Error("error persisting leaf certificates")
		return
	}
}

// Manage is the main run loop of the manager and should be run in a go routine.
// It should be passed a cancellable context that signals when the manager should
// stop and return. If it receives an unexpected error the loop exits.
func (c *CertManager) Manage(ctx context.Context) error {
	c.logger.Trace("running cert manager")

	rootWatch, err := watch.Parse(map[string]interface{}{
		"type": "connect_roots",
	})
	if err != nil {
		return err
	}
	c.rootWatch = rootWatch
	c.rootWatch.HybridHandler = c.handleRootWatch

	leafWatch, err := watch.Parse(map[string]interface{}{
		"type":    "connect_leaf",
		"service": c.service,
	})
	if err != nil {
		return err
	}
	c.leafWatch = leafWatch
	c.leafWatch.HybridHandler = c.handleLeafWatch

	wrapWatch := func(w *watch.Plan) {
		if err := w.RunWithClientAndHclog(c.consul, c.logger); err != nil {
			c.logger.Error("consul watch.Plan returned unexpectedly", "error", err)
		}
		c.logger.Trace("consul watch.Plan stopped")
	}

	// Consul 1.11 has a bug where blocking queries on the leaf certificate endpoint
	// cause all subsequent non-blocking queries to unexpectedly block. The problem
	// is that this means that, on restart, the query for a leaf certificate with
	// the given service id will never return until the previous leaf certificate
	// expires/is rotated. Adding a wait here causes the API to return once the timeout has
	// been hit -- allowing us to short-circuit the buggy blocking. The subsequent
	// goroutines can then be leveraged to pick up any certificate rotations.
	if !c.skipExtraFetch {
		leafCert, _, err := c.consul.Agent().ConnectCALeaf(c.service, &api.QueryOptions{
			WaitTime: 1 * time.Second,
		})
		if err != nil {
			c.logger.Error("error grabbing leaf certificate", "error", err)
			return err
		}
		c.handleLeafWatch(nil, leafCert)
	}
	go wrapWatch(c.rootWatch)
	go wrapWatch(c.leafWatch)

	<-ctx.Done()
	c.rootWatch.Stop()
	c.leafWatch.Stop()

	return nil
}

// only call persist when the mutex lock is held
func (c *CertManager) persist() error {
	rootCAFile := path.Join(c.directory, RootCAFile)
	clientCertFile := path.Join(c.directory, ClientCertFile)
	clientPrivateKeyFile := path.Join(c.directory, ClientPrivateKeyFile)

	if c.directory != "" {
		if c.ca != nil {
			c.logger.Trace("writing root CA file", "file", rootCAFile)
			if err := os.WriteFile(rootCAFile, c.ca, 0600); err != nil {
				c.logger.Error("error writing root CA file", "error", err)
				return fmt.Errorf("error writing root CA fiile: %w", err)
			}
		}
		if c.certificate != nil {
			c.logger.Trace("writing client cert file", "file", clientCertFile)
			if err := os.WriteFile(clientCertFile, c.certificate, 0600); err != nil {
				c.logger.Error("error writing client cert file", "error", err)
				return fmt.Errorf("error writing client cert fiile: %w", err)
			}
		}
		if c.privateKey != nil {
			c.logger.Trace("writing client private key file", "file", clientPrivateKeyFile)
			if err := os.WriteFile(clientPrivateKeyFile, c.privateKey, 0600); err != nil {
				c.logger.Error("error writing client private key file", "error", err)
				return fmt.Errorf("error writing client private key fiile: %w", err)
			}
		}
	}

	c.signal()

	return nil
}

func (c *CertManager) signal() {
	isInitialized := c.ca != nil && c.certificate != nil && c.privateKey != nil

	if !c.signalled && isInitialized {
		close(c.initializeSignal)
		c.signalled = true
	}
}

// RootCA returns the current CA cert
func (c *CertManager) RootCA() []byte {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.ca
}

// RootPool returns the certificate pool for the connect root CA
func (c *CertManager) RootPool() *x509.CertPool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.rootCertificatePool
}

func (c *CertManager) SPIFFE() *url.URL {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.spiffeURL
}

// Certificate returns the current leaf cert
func (c *CertManager) Certificate() []byte {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.certificate
}

// PrivateKey returns the current leaf cert private key
func (c *CertManager) PrivateKey() []byte {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.privateKey
}

// TLSCertificate returns the current leaf certificate as a parsed structure
func (c *CertManager) TLSCertificate() *tls.Certificate {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.tlsCertificate
}

// WaitForWrite acts as a signalling mechanism for when the certificates are
// written to disk. It is intended to be used for use-cases where initial certificates
// must be in place prior to being referenced by a consumer.
func (c *CertManager) WaitForWrite(ctx context.Context) error {
	select {
	case <-c.initializeSignal:
		return nil
	case <-ctx.Done():
		return errors.New("wait canceled")
	}
}

func (c *CertManager) RenderSDSConfig() (string, error) {
	if c.directory == "" {
		return "", errors.New("CertManager must be configured to persist to a directory")
	}

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
	if err := os.WriteFile(path.Join(c.configDirectory, SDSCertConfigFile), sdsCertConfig.Bytes(), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(path.Join(c.configDirectory, SDSCAConfigFile), sdsCAConfig.Bytes(), 0600); err != nil {
		return "", err
	}

	// now render out the json for the sds config itself and return it

	if err := sdsClusterTemplate.Execute(&sdsConfig, &sdsClusterArgs{
		Name:              "sds-cluster",
		CertSDSConfigPath: sdsCertConfigPath,
		CASDSConfigPath:   sdsCAConfigPath,
		AddressType:       common.AddressTypeForAddress(c.sdsAddress),
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
