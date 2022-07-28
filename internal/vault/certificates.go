package vault

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/armon/go-metrics"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"

	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	gwMetrics "github.com/hashicorp/consul-api-gateway/internal/metrics"
)

var _ envoy.SecretClient = (*SecretClient)(nil)

const (
	SecretScheme = "vault"

	defaultIssuer = "default"
)

// SecretClient acts as a secret fetcher for Vault.
//
// This Vault-specific implementation corresponds with the K8s-specific
// implementation, k8s.K8sSecretClient.
type SecretClient struct {
	logger hclog.Logger
	client *api.Client

	pkiPath string
	issuer  string
	issue   string

	cache     map[string]certutil.ParsedCertBundle
	cacheLock sync.RWMutex
}

// NewSecretClient relies on having standard VAULT_x envars set
// such as VAULT_TOKEN, VAULT_ADDR, etc.
func NewSecretClient(logger hclog.Logger, config *api.Config, pkiPath, issue string) (*SecretClient, error) {
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	// Ensure no leading or trailing / for path interpolation later
	pkiPath = strings.Trim(pkiPath, "/")

	return &SecretClient{
		logger:  logger,
		client:  client,
		pkiPath: pkiPath,
		issuer:  defaultIssuer,
		issue:   issue,
		cache:   make(map[string]certutil.ParsedCertBundle),
	}, nil
}

// FetchSecret accepts an opaque string containing necessary values for retrieving
// a certificate and private key from Vault. It retrieves the certificate and private
// key, stores them in memory, and returns a tls.Secret acceptable for Envoy SDS.
func (c *SecretClient) FetchSecret(ctx context.Context, fullName string) (*tls.Secret, time.Time, error) {
	c.cacheLock.RLock()
	cert, cached := c.cache[fullName]
	c.cacheLock.RUnlock()
	if !cached {
		generatedCert, err := c.generateCertBundle(ctx, fullName)
		if err != nil {
			return nil, time.Time{}, err
		}
		c.cacheLock.Lock()
		c.cache[fullName] = *generatedCert
		c.cacheLock.Unlock()
	}

	return &tls.Secret{
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: cert.CertificateBytes,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: cert.PrivateKeyBytes,
					},
				},
			},
		},
		Name: fullName,
	}, cert.Certificate.NotAfter, nil
}

func (c *SecretClient) generateCertBundle(ctx context.Context, fullName string) (*certutil.ParsedCertBundle, error) {
	// TODO Determine proper format, necessary values for body below
	_, err := ParseSecret(fullName)
	if err != nil {
		return nil, err
	}

	c.logger.Trace("fetching SDS secret", "name", fullName)
	gwMetrics.Registry.IncrCounterWithLabels(gwMetrics.SDSCertificateFetches, 1, []metrics.Label{
		{Name: "fetcher", Value: "vault"},
		{Name: "name", Value: fullName}})

	// Generate certificate + key using Vault API
	// https://www.vaultproject.io/api-docs/secret/pki#generate-certificate-and-key
	path := fmt.Sprintf("/v1/%s/issuer/%s/issue/%s", c.pkiPath, c.issuer, c.issue)

	body := map[string]interface{}{
		// TODO Pass along values from parsed fullName above
	}

	secret, err := c.client.Logical().WriteWithContext(ctx, path, body)
	if err != nil {
		return nil, err
	}

	certBundle, err := certutil.ParsePKIMap(secret.Data)
	if err != nil {
		return nil, err
	}

	// TODO Determine whether making invalid assumptions about encoding
	return certBundle, nil
}
