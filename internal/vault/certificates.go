package vault

import (
	"context"
	"time"

	"github.com/armon/go-metrics"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"

	gwMetrics "github.com/hashicorp/consul-api-gateway/internal/metrics"
)

const SecretScheme = "vault"

// SecretClient acts as a secret fetcher for Vault.
//
// This Vault-specific implementation corresponds with the K8s-specific
// implementation, k8s.K8sSecretClient.
type SecretClient struct {
	logger hclog.Logger
	client *api.Client
}

func NewSecretClient(logger hclog.Logger, config *api.Config) (*SecretClient, error) {
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &SecretClient{
		logger: logger,
		client: client,
	}, nil
}

// FetchSecret accepts an opaque string containing necessary values for retrieving
// a certificate and private key from Vault. It retrieves the certificate and private
// key, stores them in memory, and returns a tls.Secret acceptable for Envoy SDS.
func (c *SecretClient) FetchSecret(ctx context.Context, fullName string) (*tls.Secret, time.Time, error) {
	parsedSecret, err := ParseSecret(fullName)
	if err != nil {
		return nil, time.Time{}, err
	}

	c.logger.Trace("fetching SDS secret", "name", fullName)
	gwMetrics.Registry.IncrCounterWithLabels(gwMetrics.SDSCertificateFetches, 1, []metrics.Label{
		{Name: "fetcher", Value: "vault"},
		{Name: "name", Value: parsedSecret.String()}}) // TODO Use fullName instead of serializing again

	// TODO Retrieve certificate + key from Vault

	// TODO Convert to *tls.Secret

	// TODO Return *tls.Secret, x509.Certificate.NotAfter
	return &tls.Secret{}, time.Now(), nil
}
