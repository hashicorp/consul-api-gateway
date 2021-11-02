package envoy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyTLS "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryservice "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/consul-api-gateway/internal/envoy/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/store/memory"
	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
	"github.com/hashicorp/go-hclog"
)

func TestSDSRunCertificateVerification(t *testing.T) {
	t.Parallel()

	ca, server, client := gwTesting.DefaultCertificates()

	err := runTestServer(t, ca.CertBytes, func(ctrl *gomock.Controller) GatewaySecretRegistry {
		gatewayRegistry := mocks.NewMockGatewaySecretRegistry(ctrl)
		gatewayRegistry.EXPECT().GatewayExists(gomock.Any(), gomock.Any()).MinTimes(1).Return(true, nil)
		gatewayRegistry.EXPECT().CanFetchSecrets(gomock.Any(), gomock.Any(), gomock.Any()).MinTimes(1).Return(true, nil)
		return gatewayRegistry
	}, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().MinTimes(1).Return(&server.X509)

		err := testClientSDS(t, serverAddress, client, ca.CertBytes)
		require.NoError(t, err)
	})
	require.NoError(t, err)
}

func TestSDSRunServerParseError(t *testing.T) {
	t.Parallel()

	ca, _, client := gwTesting.DefaultCertificates()
	newCA, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		IsCA: true,
	})
	require.NoError(t, err)
	server, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		CA:          newCA,
		ServiceName: "server",
	})
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().MinTimes(1).Return(&server.X509)

		err := testClientSDS(t, serverAddress, client, ca.CertBytes)

		// error on invalid server certificate
		require.Error(t, err)
		require.Contains(t, status.Convert(err).String(), "x509: certificate signed by unknown authority")
	})
	require.NoError(t, err)
}

func TestSDSRunClientVerificationError(t *testing.T) {
	t.Parallel()

	ca, server, _ := gwTesting.DefaultCertificates()
	newCA, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		IsCA: true,
	})
	require.NoError(t, err)
	client, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		CA:          newCA,
		ServiceName: "client",
	})
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().MinTimes(1).Return(&server.X509)

		err := testClientSDS(t, serverAddress, client, ca.CertBytes)

		// error on invalid client private key
		require.Error(t, err)
		// connection closed or error happens on a bad client cert
		require.Truef(t,
			strings.Contains(err.Error(), "connection closed") || strings.Contains(err.Error(), "connection error"),
			"error message should have contained 'connection closed' or 'connection error', but got: %v", err)

	})
	require.NoError(t, err)
}

func TestSDSSPIFFEHostMismatch(t *testing.T) {
	t.Parallel()

	ca, server, _ := gwTesting.DefaultCertificates()
	client, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		CA:                 ca,
		ServiceName:        "client",
		SPIFFEHostOverride: "mismatch.consul",
	})
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)
		err := testClientSDS(t, serverAddress, client, ca.CertBytes)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to authenticate request")
	})
	require.NoError(t, err)
}

func TestSDSSPIFFEPathParsing(t *testing.T) {
	t.Parallel()

	ca, server, _ := gwTesting.DefaultCertificates()
	client, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		CA:                 ca,
		ServiceName:        "client",
		SPIFFEPathOverride: "/invalid/path",
	})
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)
		err := testClientSDS(t, serverAddress, client, ca.CertBytes)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to authenticate request")
	})
	require.NoError(t, err)
}

func TestSDSSPIFFEPathParsingFieldMismatch(t *testing.T) {
	t.Parallel()

	ca, server, _ := gwTesting.DefaultCertificates()
	client, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		CA:                 ca,
		ServiceName:        "client",
		SPIFFEPathOverride: "/ns/1/dc/2/something/3",
	})
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)
		err := testClientSDS(t, serverAddress, client, ca.CertBytes)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to authenticate request")
	})
	require.NoError(t, err)
}

func TestSDSSPIFFENoMatchingGateway(t *testing.T) {
	t.Parallel()

	ca, server, client := gwTesting.DefaultCertificates()

	err := runTestServer(t, ca.CertBytes, func(ctrl *gomock.Controller) GatewaySecretRegistry {
		gatewayRegistry := mocks.NewMockGatewaySecretRegistry(ctrl)
		gatewayRegistry.EXPECT().GatewayExists(gomock.Any(), gomock.Any()).MinTimes(1).Return(false, nil)
		return gatewayRegistry
	}, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)
		err := testClientSDS(t, serverAddress, client, ca.CertBytes)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to authenticate request")
	})
	require.NoError(t, err)
}

func testClientSDS(t *testing.T, address string, cert *gwTesting.CertificateInfo, ca []byte) error {
	t.Helper()

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca) {
		t.Fatal("failed to add server CA's certificate")
	}

	var err error
	var connection *grpc.ClientConn
	err = retryRequest(func(ctx context.Context) error {
		connection, err = grpc.DialContext(ctx, address, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName:   "127.0.0.1",
			Certificates: []tls.Certificate{cert.X509},
			RootCAs:      certPool,
		})))
		return err
	})
	if err != nil {
		return err
	}
	return retryRequest(func(ctx context.Context) error {
		client, err := secretservice.NewSecretDiscoveryServiceClient(connection).StreamSecrets(context.Background())
		if err != nil {
			return err
		}
		err = client.Send(&discoveryservice.DiscoveryRequest{
			Node: &core.Node{
				Id: uuid.New().String(),
			},
			ResourceNames: []string{"test"},
			TypeUrl:       resource.SecretType,
		})
		if err != nil {
			return err
		}
		_, err = client.Recv()
		return err
	})
}

func runTestServer(t *testing.T, ca []byte, registryFn func(*gomock.Controller) GatewaySecretRegistry, callback func(serverAddress string, fetcher *mocks.MockCertificateFetcher)) error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	directory, err := os.MkdirTemp("", "consul-api-gateway-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)
	socketPath := path.Join(directory, "sds.sock")

	serverAddress := socketPath
	// test over unix sockets because free ports are flaky
	connectionAddress := "unix://" + serverAddress
	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	secretClient := mocks.NewMockSecretClient(ctrl)

	block, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	certPool := x509.NewCertPool()
	certPool.AddCert(caCert)
	spiffe := caCert.URIs[0]

	fetcher.EXPECT().SPIFFE().AnyTimes().Return(spiffe)
	fetcher.EXPECT().RootPool().AnyTimes().Return(certPool)
	secretClient.EXPECT().FetchSecret(gomock.Any(), "test").AnyTimes().Return(&envoyTLS.Secret{
		Name: "test",
	}, time.Now(), nil)

	sds := NewSDSServer(hclog.NewNullLogger(), fetcher, secretClient, memory.NewStore(memory.StoreConfig{}))
	sds.bindAddress = serverAddress
	sds.protocol = "unix"
	if registryFn != nil {
		sds.gatewayRegistry = registryFn(ctrl)
	}

	errEarlyTestTermination := errors.New("early termination")
	done := make(chan error, 1)
	go func() {
		defer func() {
			// write an error to the channel, if
			// the server canceled successfully the err will be nil
			// and the read will get that first, this will only
			// be read if we have some early expectation that calls
			// runtime.Goexit prior to the server stopping
			done <- errEarlyTestTermination
		}()
		done <- sds.Run(ctx)
	}()
	// wait until the server socket exists
	err = retryRequest(func(_ context.Context) error {
		_, err := os.Stat(serverAddress)
		return err
	})
	require.NoError(t, err)

	if callback != nil {
		func() {
			defer cancel()

			callback(connectionAddress, fetcher)
		}()
	}
	err = <-done
	if err != nil {
		require.NotErrorIs(t, err, errEarlyTestTermination)
	}
	return err
}

func retryRequest(retry func(ctx context.Context) error) error {
	return backoff.Retry(func() error {
		ctx, timeoutCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer timeoutCancel()

		err := retry(ctx)

		// grpc errors don't wrap errors normally, so just check the error text
		// deadline exceeded == canceled context
		// connection refused == no open port
		// EOF == no response yet on a stream
		// check for file existence if it's a unix socket
		if err != nil && (strings.Contains(err.Error(), "deadline exceeded") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "EOF") ||
			os.IsNotExist(err)) {
			return err
		}
		return backoff.Permanent(err)
		// try for up to 5 seconds
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 500))
}
