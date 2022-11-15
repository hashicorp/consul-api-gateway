package consul

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"text/template"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"

	"github.com/hashicorp/consul/proto-public/pbconnectca"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
	Addresses         []string
	GRPCPort          int
	GRPCTLS           *tls.Config
	GRPCUseTLS        bool
	Directory         string
	Namespace         string
	Partition         string
	Datacenter        string
	PrimaryDatacenter string
	SDSAddress        string
	SDSPort           int
}

// DefaultCertManagerOptions returns the default options for a CertManager instance.
func DefaultCertManagerOptions() *CertManagerOptions {
	return &CertManagerOptions{
		GRPCPort:   8502,
		Namespace:  "default",
		Partition:  "default",
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
	apiClient  Client
	grpcClient pbconnectca.ConnectCAServiceClient
	logger     hclog.Logger

	addresses         []string
	grpcPort          int
	grpcTLS           *tls.Config
	grpcUseTLS        bool
	service           string
	directory         string
	configDirectory   string // only used for testing
	namespace         string
	partition         string
	datacenter        string
	primaryDatacenter string
	sdsAddress        string
	sdsPort           int

	mutex sync.RWMutex

	signalled        bool
	initializeSignal chan struct{}

	// cached values
	ca                  []byte
	trustDomain         string
	certificate         []byte
	privateKey          []byte
	tlsCertificate      *tls.Certificate
	rootCertificatePool *x509.CertPool

	// watches
	// rootWatch *watch.Plan
	leafWatch *watch.Plan

	// these can be overwritten to modify retry logic in testing
	writeCerts     certWriter
	skipExtraFetch bool
}

// NewCertManager creates a new CertManager instance.
func NewCertManager(logger hclog.Logger, apiClient Client, service string, options *CertManagerOptions) *CertManager {
	if options == nil {
		options = DefaultCertManagerOptions()
	}
	manager := &CertManager{
		addresses:         options.Addresses,
		grpcPort:          options.GRPCPort,
		grpcTLS:           options.GRPCTLS,
		grpcUseTLS:        options.GRPCUseTLS,
		apiClient:         apiClient,
		logger:            logger,
		namespace:         options.Namespace,
		partition:         options.Partition,
		datacenter:        options.Datacenter,
		primaryDatacenter: options.PrimaryDatacenter,
		sdsAddress:        options.SDSAddress,
		sdsPort:           options.SDSPort,
		service:           service,
		configDirectory:   options.Directory,
		directory:         options.Directory,
		initializeSignal:  make(chan struct{}),
	}
	manager.writeCerts = manager.persist
	return manager
}

func (c *CertManager) watchRoots(ctx context.Context, rotatedRootCh chan *pbconnectca.WatchRootsResponse) error {
	c.logger.Trace("starting CA roots watch stream")

	// This doesn't appear to allow specifying a primary datacenter, unclear if
	// it gets forwarded automatically or the primary must be dialed directly.
	stream, err := c.grpcClient.WatchRoots(ctx, &pbconnectca.WatchRootsRequest{})
	if err != nil {
		c.logger.Error(err.Error())
		return err
	}
	for {
		root, err := stream.Recv()
		if err != nil {
			c.logger.Error(err.Error())
			return err
		}
		select {
		case rotatedRootCh <- root:
		case <-ctx.Done():
			return nil
		}
	}
}

func (c *CertManager) handleRootWatch(ctx context.Context, response *pbconnectca.WatchRootsResponse) {
	c.logger.Trace("handling CA roots response")

	// TODO: this shouldn't ever really happen?
	if response == nil {
		c.logger.Error("received nil interface")
		return // ignore
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Set trust domain
	c.trustDomain = response.TrustDomain
	c.logger.Trace("setting trust domain")

	roots := x509.NewCertPool()
	id := response.ActiveRootId
	foundActiveRoot := false
	for _, root := range response.Roots {
		// Add all roots, including non-active, to root certificate pool
		roots.AppendCertsFromPEM([]byte(root.RootCert))

		// Set active root as CA
		if root.Id == id {
			c.logger.Trace("found active root")
			foundActiveRoot = true
			c.ca = []byte(root.RootCert)
		}
	}

	if !foundActiveRoot {
		c.logger.Error("active root not found from root watch")
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

	c.mutex.Lock()
	defer c.mutex.Unlock()

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

	grpcAddress := fmt.Sprintf("%s:%d", c.addresses[0], c.grpcPort)

	c.logger.Trace("dialing " + grpcAddress)
	c.logger.Trace("tls", c.grpcUseTLS)

	// Default to insecure credentials unless TLS config has been provided
	tlsCredentials := insecure.NewCredentials()
	if c.grpcUseTLS {
		c.logger.Trace(fmt.Sprintf("configuring gRPC TLS credentials: %+v", c.grpcTLS))
		tlsCredentials = credentials.NewTLS(c.grpcTLS)
	}

	conn, err := grpc.DialContext(ctx, grpcAddress, grpc.WithTransportCredentials(tlsCredentials))
	if err != nil {
		c.logger.Error(err.Error())
		return err
	}

	c.grpcClient = pbconnectca.NewConnectCAServiceClient(conn)

	// TODO: should this move to a field on the CertManager struct?
	rotatedRootCh := make(chan *pbconnectca.WatchRootsResponse)

	// TODO: does this need to be stopped/cleaned up at some point, or will
	// the context handle that?
	// TODO: is there a good reason to wrap these in an errgroup?
	go c.watchRoots(ctx, rotatedRootCh)

	// Don't try to pull from the channel until after the watch has started
	go func() error {
		for {
			select {
			case r := <-rotatedRootCh:
				c.handleRootWatch(ctx, r)
				// TODO: generate and sign new leaf certificate
				// expiration, err = c.fetchCert(ctx)
			case <-ctx.Done():
				return nil
			}
		}
	}()

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
		if err := w.RunWithClientAndHclog(c.apiClient.Internal(), c.logger); err != nil {
			c.logger.Error("consul watch.Plan returned unexpectedly", "error", err)
		}
		c.logger.Trace("consul watch.Plan stopped")
	}

	// go wrapWatch(c.rootWatch)
	go wrapWatch(c.leafWatch)

	<-ctx.Done()
	// c.rootWatch.Stop()
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
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.ca
}

// RootPool returns the certificate pool for the connect root CA
func (c *CertManager) RootPool() *x509.CertPool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.rootCertificatePool
}

// Certificate returns the current leaf cert
func (c *CertManager) Certificate() []byte {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.certificate
}

// PrivateKey returns the current leaf cert private key
func (c *CertManager) PrivateKey() []byte {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.privateKey
}

// TLSCertificate returns the current leaf certificate as a parsed structure
func (c *CertManager) TLSCertificate() *tls.Certificate {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

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
