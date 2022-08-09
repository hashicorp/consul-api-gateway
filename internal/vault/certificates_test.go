package vault

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/certutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/vault/mocks"
)

func TestNewPKISecretClient(t *testing.T) {
	client, err := NewPKISecretClient(hclog.NewNullLogger(), "/pki/", t.Name())
	require.NoError(t, err)
	assert.NotNil(t, client.client)
	assert.Equal(t, "pki", client.pkiPath)
	assert.Equal(t, defaultIssuer, client.issuer)
	assert.Equal(t, t.Name(), client.issue)
}

func TestPKISecretClient_FetchSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client, err := NewPKISecretClient(hclog.NewNullLogger(), "pki", t.Name())
	require.NoError(t, err)

	ttl := 12 * time.Hour
	vaultSecret := NewPKISecret("example.com", "", "", "", ttl.String())

	cert, key := generateCertAndKey(t, "example.com", ttl)

	vault := mocks.NewMockLogicalClient(ctrl)
	vault.EXPECT().
		WriteWithContext(
			context.TODO(),
			fmt.Sprintf("/%s/issuer/%s/issue/%s", client.pkiPath, defaultIssuer, t.Name()),
			map[string]interface{}{
				"alt_names":   "",
				"common_name": vaultSecret.CommonName,
				"csr":         "",
				"ip_sans":     "",
				"other_sans":  "",
				"ou":          "",
				"ttl":         vaultSecret.TTL,
			}).
		Return(&api.Secret{
			Data: map[string]interface{}{
				"ca_chain":         []string{string(cert)},
				"certificate":      string(cert),
				"issuing_ca":       string(cert),
				"private_key":      string(key),
				"private_key_type": certutil.RSAPrivateKey,
				"serial_number":    "2022",
			},
		}, nil)
	client.client = vault

	tlsSecret, notAfter, err := client.FetchSecret(context.TODO(), vaultSecret.String())
	require.NoError(t, err)
	require.NotNil(t, tlsSecret)
	assert.True(t, notAfter.After(time.Now().Add(11*time.Hour))) // Should be *about* 12 hours

	require.NotNil(t, tlsSecret.Type)
	stc, ok := tlsSecret.Type.(*tlsv3.Secret_TlsCertificate)
	require.True(t, ok)
	require.NotNil(t, stc.TlsCertificate)

	// Verify cert chain present
	require.NotNil(t, stc.TlsCertificate.CertificateChain)
	require.NotNil(t, stc.TlsCertificate.CertificateChain.Specifier)
	ccs, ok := stc.TlsCertificate.CertificateChain.Specifier.(*core.DataSource_InlineString)
	require.True(t, ok)
	assert.NotEmpty(t, ccs.InlineString)

	// Verify private key present
	require.NotNil(t, stc.TlsCertificate.PrivateKey)
	require.NotNil(t, stc.TlsCertificate.PrivateKey.Specifier)
	pks, ok := stc.TlsCertificate.PrivateKey.Specifier.(*core.DataSource_InlineBytes)
	require.True(t, ok)
	assert.NotEmpty(t, pks.InlineBytes)
}

func generateCertAndKey(t *testing.T, commonName string, ttl time.Duration) ([]byte, []byte) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2022),
		Subject: pkix.Name{
			Organization:  []string{"Hashicorp Inc."},
			Country:       []string{"USA"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"101 2nd St #700"},
			PostalCode:    []string{"94105"},
			CommonName:    commonName,
		},
		NotAfter:              time.Now().Add(ttl),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivateKey.PublicKey, caPrivateKey)
	require.NoError(t, err)

	caPEM := new(bytes.Buffer)
	require.NoError(t, pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	}))

	caPrivKeyPEM := new(bytes.Buffer)
	require.NoError(t, pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivateKey),
	}))

	return caPEM.Bytes(), caPrivKeyPEM.Bytes()
}
