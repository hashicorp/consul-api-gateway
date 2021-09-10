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
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/polar/internal/metrics"
)

type K8sSecretClient struct {
	logger  hclog.Logger
	client  client.Client
	counter *prometheus.CounterVec
}

func NewK8sSecretClient(logger hclog.Logger, metrics *metrics.SDSMetrics) (*K8sSecretClient, error) {
	apiClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	return &K8sSecretClient{
		logger:  logger,
		client:  apiClient,
		counter: metrics.CertificateFetches.MustCurryWith(prometheus.Labels{"fetcher": "k8s"}),
	}, nil
}

func (c *K8sSecretClient) FetchSecret(ctx context.Context, fullName string) (*tls.Secret, time.Time, error) {
	c.logger.Trace("fetching SDS secret", "name", fullName)
	c.counter.WithLabelValues(fullName).Inc()
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
