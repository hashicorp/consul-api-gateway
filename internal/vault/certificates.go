package vault

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
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
var _ envoy.SecretClient = (*KVSecretClient)(nil)

const (
	PKISecretScheme    = "vault+pki"
	StaticSecretScheme = "vault+kv"

	defaultIssuer = "default"
)

//go:generate mockgen -source ./certificates.go -destination ./mocks/certificates.go -package mocks LogicalClient
type LogicalClient interface {
	WriteWithContext(context.Context, string, map[string]interface{}) (*api.Secret, error)
}

//go:generate mockgen -source ./certificates.go -destination ./mocks/certificates.go -package mocks KVClient
type KVClient interface {
	Get(context.Context, string) (*api.KVSecret, error)
}

// KVSecretClient acts as a certificate retriever using Vault's KV store.
//
// This Vault-specific implementation corresponds with the K8s-specific
// implementation, k8s.K8sSecretClient.
type KVSecretClient struct {
	logger hclog.Logger
	client KVClient
}

// NewKVSecretClient relies on having standard VAULT_x envars set
// such as VAULT_TOKEN, VAULT_ADDR, etc. In the future, we may need
// to construct the config externally to allow for custom flags, etc.
func NewKVSecretClient(logger hclog.Logger, kvPath string) (*KVSecretClient, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}

	// Ensure no leading or trailing / for path interpolation later
	kvPath = strings.Trim(kvPath, "/")

	return &KVSecretClient{
		logger: logger,
		client: client.KVv2(kvPath),
	}, nil
}

// FetchSecret accepts an opaque string containing necessary values for retrieving
// a certificate and private key from Vault KV. It retrieves the certificate and private
// key, stores them in memory, and returns a tls.Secret acceptable for Envoy SDS.
func (c *KVSecretClient) FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error) {
	secret, err := ParseKVSecret(name)
	if err != nil {
		return nil, time.Time{}, err
	}

	c.logger.Trace("fetching SDS secret", "name", name)
	gwmetrics.Registry.IncrCounterWithLabels(gwmetrics.SDSCertificateFetches, 1, []metrics.Label{
		{Name: "fetcher", Value: StaticSecretScheme},
		{Name: "name", Value: name}})

	path := strings.TrimPrefix(secret.Path, "/")

	// Fetch
	result, err := c.client.Get(ctx, path)
	if err != nil {
		return nil, time.Time{}, err
	}

	certValue, certExists := result.Data[secret.CertKey]
	privateKeyValue, privateKeyExists := result.Data[secret.PrivateKeyKey]
	if !certExists || !privateKeyExists {
		return nil, time.Time{}, errors.New("cert or private key not included in Vault secret")
	}

	// Decode and parse certificate to extract expiration
	b64Cert, ok := certValue.(string)
	if !ok || b64Cert == "" {
		return nil, time.Time{}, errors.New("cert value from Vault not string")
	}
	certBytes, err := base64.StdEncoding.DecodeString(b64Cert)
	if err != nil {
		return nil, time.Time{}, errors.New("cert value from Vault not base64-decodable")
	}
	block, _ := pem.Decode(certBytes)
	if block == nil {
		return nil, time.Time{}, errors.New("failed to parse PEM cert value from Vault")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to parse cert: %w", err)
	}

	// Decode private key
	b64PrivateKey, ok := privateKeyValue.(string)
	if !ok || b64PrivateKey == "" {
		return nil, time.Time{}, errors.New("private key value from Vault not string")
	}
	privateKeyBytes, err := base64.StdEncoding.DecodeString(b64PrivateKey)
	if err != nil {
		return nil, time.Time{}, errors.New("private key value from Vault not base64-decodable")
	}

	return &tls.Secret{
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: certBytes,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: privateKeyBytes,
					},
				},
			},
		},
	}, cert.NotAfter, nil
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

// FetchSecret accepts an opaque string containing necessary values for generating a
// certificate and private key with Vault's PKI engine. It generates the certificate
// and private key, stores them in memory, and returns a tls.Secret acceptable for Envoy SDS.
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
