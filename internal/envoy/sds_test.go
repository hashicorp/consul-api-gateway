package envoy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthservice "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/envoy/mocks"
	"github.com/hashicorp/polar/internal/metrics"
	polarTesting "github.com/hashicorp/polar/internal/testing"
)

func TestSDSRunCertificateVerification(t *testing.T) {
	t.Parallel()

	ca, server, client := polarTesting.DefaultCertificates()

	err := runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)

		err := testClientHealth(t, serverAddress, client, ca.CertBytes)
		require.NoError(t, err)
	})
	require.NoError(t, err)
}

func TestSDSRunServerParseError(t *testing.T) {
	t.Parallel()

	ca, _, client := polarTesting.DefaultCertificates()
	newCA, err := polarTesting.GenerateSignedCertificate(nil, true, "")
	require.NoError(t, err)
	server, err := polarTesting.GenerateSignedCertificate(newCA, false, "server")
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)

		err := testClientHealth(t, serverAddress, client, ca.CertBytes)

		// error on invalid server certificate
		require.Error(t, err)
		require.Contains(t, status.Convert(err).String(), "x509: certificate signed by unknown authority")
	})
	require.NoError(t, err)
}

func TestSDSRunClientVerificationError(t *testing.T) {
	t.Parallel()

	ca, server, _ := polarTesting.DefaultCertificates()
	newCA, err := polarTesting.GenerateSignedCertificate(nil, true, "")
	require.NoError(t, err)
	client, err := polarTesting.GenerateSignedCertificate(newCA, false, "client")
	require.NoError(t, err)

	err = runTestServer(t, ca.CertBytes, nil, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)

		err := testClientHealth(t, serverAddress, client, ca.CertBytes)

		// error on invalid client private key
		require.Error(t, err)
		// cnnection closed happens on a bad client cert
		require.Contains(t, err.Error(), "connection closed")
	})
	require.NoError(t, err)
}

func TestSDSNoMatchingGateway(t *testing.T) {
	t.Parallel()

	ca, server, client := polarTesting.DefaultCertificates()

	err := runTestServer(t, ca.CertBytes, func(ctrl *gomock.Controller) GatewayRegistry {
		gatewayRegistry := mocks.NewMockGatewayRegistry(ctrl)
		gatewayRegistry.EXPECT().GatewayExists(gomock.Any()).Return(false)
		return gatewayRegistry
	}, func(serverAddress string, fetcher *mocks.MockCertificateFetcher) {
		fetcher.EXPECT().TLSCertificate().Return(&server.X509)
		err := testClientHealth(t, serverAddress, client, ca.CertBytes)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to authenticate request")
	})
	require.NoError(t, err)
}

func testClientHealth(t *testing.T, address string, cert *polarTesting.CertificateInfo, ca []byte) error {
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
	healthClient := healthservice.NewHealthClient(connection)
	return retryRequest(func(ctx context.Context) error {
		_, err = healthClient.Check(ctx, &healthservice.HealthCheckRequest{})
		return err
	})
}

func runTestServer(t *testing.T, ca []byte, registryFn func(*gomock.Controller) GatewayRegistry, callback func(serverAddress string, fetcher *mocks.MockCertificateFetcher)) error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	directory, err := os.MkdirTemp("", "polar-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)
	socketPath := path.Join(directory, "sds.sock")

	serverAddress := socketPath
	// test over unix sockets because free ports are flaky
	connectionAddress := "unix://" + serverAddress
	fetcher := mocks.NewMockCertificateFetcher(ctrl)
	secretClient := mocks.NewMockSecretClient(ctrl)
	fetcher.EXPECT().RootCA().Return(ca)

	sds := NewSDSServer(hclog.NewNullLogger(), metrics.Registry.SDS, fetcher, secretClient)
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
			// the server canceled succesfully the err will be nil
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
		// check for file existence if it's a unix socket
		if err != nil && (strings.Contains(err.Error(), "deadline exceeded") ||
			strings.Contains(err.Error(), "connection refused") ||
			os.IsNotExist(err)) {
			return err
		}
		return backoff.Permanent(err)
		// try for up to 5 seconds
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 500))
}
