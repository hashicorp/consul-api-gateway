package consul

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul/api"
)

const (
	RootCAFile           = "root-ca.pem"
	ClientCertFile       = "client.crt"
	ClientPrivateKeyFile = "client.pem"

	defaultCertificateDirectory = "/certs"
	defaultSignalOnNWrites      = 1
)

// CertManagerOptions contains the optional configuration used to initialize a CertManager.
type CertManagerOptions struct {
	Directory       string
	SignalOnNWrites int32
	Tries           uint64
}

// DefaultCertManagerOptions returns the default options for a CertManager instance.
func DefaultCertManagerOptions() *CertManagerOptions {
	return &CertManagerOptions{
		Directory:       defaultCertificateDirectory,
		SignalOnNWrites: defaultSignalOnNWrites,
		Tries:           defaultMaxAttempts,
	}
}

// CertManager handles Consul leaf certificate management and certificate rotation.
// Once a leaf certificate has expired, it generates a new certificate and writes
// it to the location given in the configuration options with which it was created.
type CertManager struct {
	consul *api.Client

	service   string
	directory string

	signalWrites int32
	writesLeft   int32
	writes       chan struct{}

	tries           uint64
	backoffInterval time.Duration
}

// NewCertManager creates a new CertManager instance.
func NewCertManager(consul *api.Client, service string, options *CertManagerOptions) *CertManager {
	if options == nil {
		options = DefaultCertManagerOptions()
	}
	return &CertManager{
		consul:          consul,
		service:         service,
		directory:       options.Directory,
		signalWrites:    options.SignalOnNWrites,
		writesLeft:      options.SignalOnNWrites,
		writes:          make(chan struct{}),
		tries:           options.Tries,
		backoffInterval: defaultBackoffInterval,
	}
}

// Manage is the main run loop of the manager and should be run in a go routine.
// It should be passed a cancellable context that signals when the manager should
// stop and return. If it receives an unexpected error the loop exits.
func (c *CertManager) Manage(ctx context.Context) error {
	err := backoff.Retry(func() error {
		return c.manage(ctx)
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(c.backoffInterval), c.tries), ctx))
	if errors.Is(err, context.Canceled) {
		// we intentionally canceled the context, just return
		return nil
	}
	return err
}

func (c *CertManager) manage(ctx context.Context) error {
	for {
		options := &api.QueryOptions{}
		clientCert, _, err := c.consul.Agent().ConnectCALeaf(c.service, options.WithContext(ctx))
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// we intentionally canceled the context, just return
				return nil
			}
			return fmt.Errorf("error generating client leaf certificate: %w", err)
		}
		expiresIn := time.Until(clientCert.ValidBefore)
		roots, _, err := c.consul.Agent().ConnectCARoots(options.WithContext(ctx))
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// we intentionally canceled the context, just return
				return nil
			}
			return fmt.Errorf("error retrieving root CA: %w", err)
		}
		for _, root := range roots.Roots {
			if root.Active {
				err := c.persist(root, clientCert)
				if err != nil {
					return err
				}
				break
			}
		}
		select {
		case <-time.After(expiresIn):
			// loop
		case <-ctx.Done():
			return nil
		}
	}
}

func (c *CertManager) persist(root *api.CARoot, client *api.LeafCert) error {
	rootCAFile := path.Join(c.directory, RootCAFile)
	clientCertFile := path.Join(c.directory, ClientCertFile)
	clientPrivateKeyFile := path.Join(c.directory, ClientPrivateKeyFile)
	if err := os.WriteFile(rootCAFile, []byte(root.RootCertPEM), 0600); err != nil {
		return fmt.Errorf("error writing root CA fiile: %w", err)
	}
	if err := os.WriteFile(clientCertFile, []byte(client.CertPEM), 0600); err != nil {
		return fmt.Errorf("error writing client cert fiile: %w", err)
	}
	if err := os.WriteFile(clientPrivateKeyFile, []byte(client.PrivateKeyPEM), 0600); err != nil {
		return fmt.Errorf("error writing client private key fiile: %w", err)
	}

	writesLeft := atomic.AddInt32(&c.writesLeft, -1)
	if writesLeft >= 0 {
		c.writes <- struct{}{}
	}

	return nil
}

// Wait acts as a signalling mechanism for when the certificates are
// written to disk. It is intended to be used for use-cases where initial certificates
// must be in place prior to being referenced by a consumer.
func (c *CertManager) Wait() {
	for {
		if c.signalWrites <= 0 {
			break
		}
		<-c.writes
		c.signalWrites--
	}
}
