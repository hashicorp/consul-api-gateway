package k8s

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/envoy"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K8sSecretClient struct {
	logger hclog.Logger
	client client.Client
}

func NewK8sSecretClient(logger hclog.Logger) (*K8sSecretClient, error) {
	apiClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{
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

func (c *K8sSecretClient) FetchSecret(ctx context.Context, fullName string) (*envoy.Certificate, error) {
	c.logger.Debug("fetching SDS secret", "name", fullName)
	namespace, name := parseSecretName(fullName)
	secret := &corev1.Secret{}
	err := c.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		return nil, err
	}
	if secret.Type != corev1.SecretTypeTLS {
		return nil, fmt.Errorf("only TLS certificates are supported, got type: %s", secret.Type)
	}
	certificateChain := secret.Data[corev1.TLSCertKey]
	block, _ := pem.Decode(certificateChain)
	if block == nil {
		return nil, errors.New("failed to parse certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	certificatePrivateKey := secret.Data[corev1.TLSPrivateKeyKey]
	return &envoy.Certificate{
		Name:             fullName,
		CertificateChain: certificateChain,
		PrivateKey:       certificatePrivateKey,
		ExpiresAt:        cert.NotAfter,
	}, nil
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
