package vault

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	gwmetrics "github.com/hashicorp/consul-api-gateway/internal/metrics"
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
}

// NewSecretClient relies on having standard VAULT_x envars set
// such as VAULT_TOKEN, VAULT_ADDR, etc. In the future, we may need
// to construct the config externally to allow for custom flags, etc.
func NewSecretClient(logger hclog.Logger, pkiPath, issue string) (*SecretClient, error) {
	client, err := api.NewClient(api.DefaultConfig())
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
	}, nil
}

// FetchSecret accepts an opaque string containing necessary values for retrieving
// a certificate and private key from Vault. It retrieves the certificate and private
// key, stores them in memory, and returns a tls.Secret acceptable for Envoy SDS.
func (c *SecretClient) FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error) {
	cert, err := c.generateCertBundle(ctx, name)
	if err != nil {
		return nil, time.Time{}, err
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
		Name: name,
	}, cert.Certificate.NotAfter, nil
}

// generateCertBundle calls the Vault endpoint for generating a certificate + key
// and returns the parsed bundle.
//
// https://www.vaultproject.io/api-docs/secret/pki#generate-certificate-and-key
func (c *SecretClient) generateCertBundle(ctx context.Context, name string) (*certutil.ParsedCertBundle, error) {
	secret, err := ParseSecret(name)
	if err != nil {
		return nil, err
	}

	c.logger.Trace("fetching SDS secret", "name", name)
	gwmetrics.Registry.IncrCounterWithLabels(gwmetrics.SDSCertificateFetches, 1, []metrics.Label{
		{Name: "fetcher", Value: "vault"},
		{Name: "name", Value: name}})

	// Generate certificate + key using Vault API
	path := fmt.Sprintf("/v1/%s/issuer/%s/issue/%s", c.pkiPath, c.issuer, c.issue)

	body := make(map[string]interface{})
	if err = mapstructure.Decode(
		certutil.IssueData{
			AltNames:   secret.AltNames,
			CommonName: secret.CommonName,
			IPSANs:     secret.IPSANs,
			OtherSANs:  secret.OtherSANs,
			TTL:        secret.TTL,
		}, body); err != nil {
		return nil, err
	}

	result, err := c.client.Logical().WriteWithContext(ctx, path, body)
	if err != nil {
		return nil, err
	}

	certBundle, err := certutil.ParsePKIMap(result.Data)
	if err != nil {
		return nil, err
	}

	// TODO Determine whether making invalid assumptions about encoding.
	//   Defaults to PEM format based on endpoint docs.
	return certBundle, nil
}
