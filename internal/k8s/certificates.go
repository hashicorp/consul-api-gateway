package k8s

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/armon/go-metrics"

	"github.com/hashicorp/go-hclog"

	gwMetrics "github.com/hashicorp/consul-api-gateway/internal/metrics"
)

// K8sSecretClient acts as a secret fetcher for kubernetes secrets
type K8sSecretClient struct {
	logger hclog.Logger
	client client.Client
}

// NewK8sSecretClient initializes a K8sSecretClient instance
func NewK8sSecretClient(logger hclog.Logger, config *rest.Config) (*K8sSecretClient, error) {
	apiClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	return &K8sSecretClient{
		logger: logger,
		client: apiClient,
	}, nil
}

// FetchSecret fetches a kubernetes secret described with the url name of k8s://namespace/secret-name
func (c *K8sSecretClient) FetchSecret(ctx context.Context, fullName string) (*tls.Secret, time.Time, error) {
	c.logger.Trace("fetching SDS secret", "name", fullName)
	gwMetrics.Registry.IncrCounterWithLabels(gwMetrics.SDSCertificateFetches, 1, []metrics.Label{{
		Name:  "fetcher",
		Value: "k8s",
	}, {
		Name:  "name",
		Value: fullName,
	}})
	namespace, name := parseSecretName(fullName)
	secret := &corev1.Secret{}
	err := c.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		return nil, time.Time{}, err
	}
	if secret.Type != corev1.SecretTypeTLS {
		return nil, time.Time{}, fmt.Errorf("only TLS certificates are supported, got type: %s", secret.Type)
	}
	certificateChain := secret.Data[corev1.TLSCertKey]
	block, _ := pem.Decode(certificateChain)
	if block == nil {
		return nil, time.Time{}, errors.New("failed to parse certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to parse certificate: %w", err)
	}
	certificatePrivateKey := secret.Data[corev1.TLSPrivateKeyKey]
	return &tls.Secret{
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: certificateChain,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: certificatePrivateKey,
					},
				},
			},
		},
		Name: fullName,
	}, cert.NotAfter, nil
}

// parses the string into a namespace and name
func parseSecretName(name string) (string, string) {
	namespacedName := strings.TrimPrefix(name, "k8s://")
	tokens := strings.SplitN(namespacedName, "/", 2)
	if len(tokens) != 2 {
		return "default", namespacedName
	}
	return tokens[0], tokens[1]
}
