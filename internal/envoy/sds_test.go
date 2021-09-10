package envoy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/envoy/mocks"
	"github.com/hashicorp/polar/internal/metrics"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthservice "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type certInfo struct {
	cert            *x509.Certificate
	certBytes       []byte
	privateKey      *rsa.PrivateKey
	privateKeyBytes []byte
	x509            tls.Certificate
}

var (
	errEarlyTestTermination = errors.New("early test termination")
)

func TestSDSRun(t *testing.T) {
	t.Parallel()

	port := getPort()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ca, _, _ := generateCerts(t)

	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	client := mocks.NewMockSecretClient(ctrl)
	fetcher.EXPECT().RootCA().Return(ca.certBytes, nil)

	sds := NewSDSServer(hclog.NewNullLogger(), metrics.Registry.SDS, fetcher, client)
	sds.bindAddress = fmt.Sprintf("127.0.0.1:%d", port)

	err := runTestServer(t, context.Background(), sds, func(cancel func()) { cancel() })
	require.NoError(t, err)
}

func TestSDSRunCertificateVerification(t *testing.T) {
	t.Parallel()

	port := getPort()
	serverAddress := fmt.Sprintf("127.0.0.1:%d", port)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ca, server, client := generateCerts(t)

	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	secretClient := mocks.NewMockSecretClient(ctrl)
	fetcher.EXPECT().RootCA().Return(ca.certBytes, nil)
	fetcher.EXPECT().Certificate().Return(server.certBytes, nil)
	fetcher.EXPECT().PrivateKey().Return(server.privateKeyBytes, nil)

	sds := NewSDSServer(hclog.NewNullLogger(), metrics.Registry.SDS, fetcher, secretClient)
	sds.bindAddress = serverAddress

	err := runTestServer(t, context.Background(), sds, func(cancel func()) {
		defer cancel()

		err := testClientHealth(t, serverAddress, client, ca.certBytes)

		require.NoError(t, err)
	})
	require.NoError(t, err)
}

func TestSDSRunServerCertificateError(t *testing.T) {
	t.Parallel()

	port := getPort()
	serverAddress := fmt.Sprintf("127.0.0.1:%d", port)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ca, _, client := generateCerts(t)

	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	secretClient := mocks.NewMockSecretClient(ctrl)
	fetcher.EXPECT().RootCA().Return(ca.certBytes, nil)
	fetcher.EXPECT().Certificate().Return(nil, errors.New("invalid"))

	sds := NewSDSServer(hclog.NewNullLogger(), metrics.Registry.SDS, fetcher, secretClient)
	sds.bindAddress = serverAddress

	err := runTestServer(t, context.Background(), sds, func(cancel func()) {
		defer cancel()

		err := testClientHealth(t, serverAddress, client, ca.certBytes)

		// error on invalid server certificate
		require.Error(t, err)
		require.Contains(t, status.Convert(err).String(), "tls")
	})
	require.NoError(t, err)
}

func TestSDSRunServerKeyError(t *testing.T) {
	t.Parallel()

	port := getPort()
	serverAddress := fmt.Sprintf("127.0.0.1:%d", port)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ca, server, client := generateCerts(t)

	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	secretClient := mocks.NewMockSecretClient(ctrl)
	fetcher.EXPECT().RootCA().Return(ca.certBytes, nil)
	fetcher.EXPECT().Certificate().Return(server.certBytes, nil)
	fetcher.EXPECT().PrivateKey().Return(nil, errors.New("invalid"))

	sds := NewSDSServer(hclog.NewNullLogger(), metrics.Registry.SDS, fetcher, secretClient)
	sds.bindAddress = serverAddress

	err := runTestServer(t, context.Background(), sds, func(cancel func()) {
		defer cancel()

		err := testClientHealth(t, serverAddress, client, ca.certBytes)

		// error on invalid server private key
		require.Error(t, err)
		require.Contains(t, status.Convert(err).String(), "tls")
	})
	require.NoError(t, err)
}

func TestSDSRunClientVerificationError(t *testing.T) {
	t.Parallel()

	port := getPort()
	serverAddress := fmt.Sprintf("127.0.0.1:%d", port)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ca, server, _ := generateCerts(t)
	_, _, client := generateCerts(t)

	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	secretClient := mocks.NewMockSecretClient(ctrl)
	fetcher.EXPECT().RootCA().Return(ca.certBytes, nil)
	fetcher.EXPECT().Certificate().Return(server.certBytes, nil)
	fetcher.EXPECT().PrivateKey().Return(server.privateKeyBytes, nil)

	sds := NewSDSServer(hclog.NewNullLogger(), metrics.Registry.SDS, fetcher, secretClient)
	sds.bindAddress = serverAddress

	err := runTestServer(t, context.Background(), sds, func(cancel func()) {
		defer cancel()

		err := testClientHealth(t, serverAddress, client, ca.certBytes)

		// error on invalid client private key
		require.Error(t, err)
		require.Contains(t, status.Convert(err).String(), "connection closed")
	})
	require.NoError(t, err)
}

func testClientHealth(t *testing.T, address string, cert *certInfo, ca []byte) error {
	t.Helper()

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca) {
		t.Fatal("failed to add server CA's certificate")
	}

	var err error
	var connection *grpc.ClientConn
	err = backoff.Retry(func() error {
		ctx, timeoutCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer timeoutCancel()
		config := credentials.NewTLS(&tls.Config{
			ServerName:   "127.0.0.1",
			Certificates: []tls.Certificate{cert.x509},
			RootCAs:      certPool,
		})

		connection, err = grpc.DialContext(ctx, address, grpc.WithTransportCredentials(config))
		if err != nil && (errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), "connection refused")) {
			return err
		}
		return backoff.Permanent(err)
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 100))
	if err != nil {
		return err
	}
	healthClient := healthservice.NewHealthClient(connection)
	err = backoff.Retry(func() error {
		ctx, timeoutCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer timeoutCancel()
		_, err = healthClient.Check(ctx, &healthservice.HealthCheckRequest{})
		if err != nil && (errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), "connection refused")) {
			return err
		}
		return backoff.Permanent(err)
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 100))
	if err != nil {
		return err
	}
	return err
}

func runTestServer(t *testing.T, ctx context.Context, sds *SDSServer, callback func(cancel func())) error {
	t.Helper()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		defer func() {
			// write an error to the channel, if
			// the server canceled succesfully the err will be nil
			// and the read will get that first, this will only
			// be read if we have some early expectation that calls
			// runtime.Goexit prior to the server stopping
			done <- errEarlyTestTermination
		}()
		done <- sds.Run(ctx)
	}()
	if callback != nil {
		callback(cancel)
	}
	err := <-done
	if err != nil {
		require.NotErrorIs(t, err, errEarlyTestTermination)
	}
	return err
}

func getSignedCert(t *testing.T, ca *certInfo) *certInfo {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Testing, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Fake Street"},
			PostalCode:    []string{"11111"},
		},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SubjectKeyId:          []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	if ca == nil {
		cert.IsCA = true
	}
	caCert := cert
	if ca != nil {
		caCert = ca.cert
	}
	caPrivateKey := privateKey
	if ca != nil {
		caPrivateKey = ca.privateKey
	}
	data, err := x509.CreateCertificate(rand.Reader, cert, caCert, &privateKey.PublicKey, caPrivateKey)
	require.NoError(t, err)

	var certificatePEM bytes.Buffer
	var privateKeyPEM bytes.Buffer
	pem.Encode(&certificatePEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	})
	pem.Encode(&privateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	certBytes := certificatePEM.Bytes()
	privateKeyBytes := privateKeyPEM.Bytes()
	x509Cert, err := tls.X509KeyPair(certBytes, privateKeyBytes)
	require.NoError(t, err)

	return &certInfo{
		cert:            cert,
		certBytes:       certBytes,
		privateKey:      privateKey,
		privateKeyBytes: privateKeyBytes,
		x509:            x509Cert,
	}
}

func generateCerts(t *testing.T) (*certInfo, *certInfo, *certInfo) {
	t.Helper()

	ca := getSignedCert(t, nil)
	server := getSignedCert(t, ca)
	client := getSignedCert(t, ca)

	return ca, server, client
}

func getPort() int {
	return freeport.MustTake(1)[0]
}
