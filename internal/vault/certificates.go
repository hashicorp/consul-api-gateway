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

var _ envoy.SecretClient = (*PKISecretClient)(nil)
var _ envoy.SecretClient = (*StaticSecretClient)(nil)

const (
	PKISecretScheme    = "vault+pki"
	StaticSecretScheme = "vault"

	defaultIssuer = "default"
)

//go:generate mockgen -source ./certificates.go -destination ./mocks/certificates.go -package mocks LogicalClient
type LogicalClient interface {
	WriteWithContext(context.Context, string, map[string]interface{}) (*api.Secret, error)
}

type StaticSecretClient struct {
	logger hclog.Logger
	client LogicalClient
}

func NewStaticSecretClient(logger hclog.Logger) (*StaticSecretClient, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}

	return &StaticSecretClient{
		logger: logger,
		client: client.Logical(),
	}, nil
}

func (c *StaticSecretClient) FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error) {
	_, err := ParseStaticSecret(name)
	if err != nil {
		return nil, time.Time{}, err
	}

	c.logger.Trace("fetching SDS secret", "name", name)
	gwmetrics.Registry.IncrCounterWithLabels(gwmetrics.SDSCertificateFetches, 1, []metrics.Label{
		{Name: "fetcher", Value: StaticSecretScheme},
		{Name: "name", Value: name}})

	// TODO Fetch certificate + key using Vault API

	// TODO Convert to *tls.Secret
	return nil, time.Time{}, nil
}

// PKISecretClient acts as a certificate generator using Vault's PKI engine.
//
// This Vault-specific implementation corresponds with the K8s-specific
// implementation, k8s.K8sSecretClient.
type PKISecretClient struct {
	logger hclog.Logger
	client LogicalClient

	pkiPath string
	issuer  string
	issue   string
}

// NewPKISecretClient relies on having standard VAULT_x envars set
// such as VAULT_TOKEN, VAULT_ADDR, etc. In the future, we may need
// to construct the config externally to allow for custom flags, etc.
func NewPKISecretClient(logger hclog.Logger, pkiPath, issue string) (*PKISecretClient, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}

	// Ensure no leading or trailing / for path interpolation later
	pkiPath = strings.Trim(pkiPath, "/")

	return &PKISecretClient{
		logger:  logger,
		client:  client.Logical(),
		pkiPath: pkiPath,
		issuer:  defaultIssuer,
		issue:   issue,
	}, nil
}

// FetchSecret accepts an opaque string containing necessary values for retrieving
// a certificate and private key from Vault. It retrieves the certificate and private
// key, stores them in memory, and returns a tls.PKISecret acceptable for Envoy SDS.
func (c *PKISecretClient) FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error) {
	cert, err := c.generateCertBundle(ctx, name)
	if err != nil {
		return nil, time.Time{}, err
	}

	bundle, err := cert.ToCertBundle()
	if err != nil {
		return nil, time.Time{}, err
	}

	if len(bundle.CAChain) == 0 {
		bundle.CAChain = []string{bundle.IssuingCA}
	}

	return &tls.Secret{
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineString{
						InlineString: strings.Join(bundle.CAChain, "\n"),
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
func (c *PKISecretClient) generateCertBundle(ctx context.Context, name string) (*certutil.ParsedCertBundle, error) {
	secret, err := ParsePKISecret(name)
	if err != nil {
		return nil, err
	}

	c.logger.Trace("fetching SDS secret", "name", name)
	gwmetrics.Registry.IncrCounterWithLabels(gwmetrics.SDSCertificateFetches, 1, []metrics.Label{
		{Name: "fetcher", Value: PKISecretScheme},
		{Name: "name", Value: name}})

	// Generate certificate + key using Vault API
	path := fmt.Sprintf("/%s/issuer/%s/issue/%s", c.pkiPath, c.issuer, c.issue)

	body := make(map[string]interface{})
	if err = mapstructure.Decode(
		certutil.IssueData{
			AltNames:   secret.AltNames,
			CommonName: secret.CommonName,
			IPSANs:     secret.IPSANs,
			OtherSANs:  secret.OtherSANs,
			TTL:        secret.TTL,
		}, &body); err != nil {
		return nil, err
	}

	result, err := c.client.WriteWithContext(ctx, path, body)
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
