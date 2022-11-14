package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/proto-public/pbconnectca"

	"github.com/hashicorp/go-hclog"

	"google.golang.org/grpc"
)

func TestManage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		leafFailures uint64
		rootFailures uint64
		expirations  uint64
	}{{
		name: "test-basic",
	}, {
		name:         "test-leaf-retries",
		leafFailures: 1,
	}, {
		name:         "test-root-retries",
		rootFailures: 1,
	}} {
		t.Run(test.name, func(t *testing.T) {
			directory, err := os.MkdirTemp("", gwTesting.RandomString())
			require.NoError(t, err)
			defer os.RemoveAll(directory)
			service := gwTesting.RandomString()

			server := runCertServer(t, service, test.leafFailures, test.rootFailures, test.expirations)

			options := DefaultCertManagerOptions()
			options.Directory = directory

			manager := NewCertManager(
				hclog.Default().Named("certmanager"),
				Config{
					Addresses: []string{server.consulAddress},
					GRPCPort:  server.consulGRPCPort,
					TLS:       nil,
				},
				NewClient(server.consulHTTPClient),
				service,
				options,
			)
			manager.skipExtraFetch = true

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			managerErr := make(chan error, 2)
			go func() {
				if err := manager.Manage(ctx); err != nil {
					managerErr <- err
				}
			}()

			finished := make(chan struct{})
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()

				if err := manager.WaitForWrite(ctx); err != nil {
					managerErr <- err
				} else {
					close(finished)
				}
			}()

			select {
			case err := <-managerErr:
				require.NoError(t, err)
			case <-finished:
			}

			rootCAFile := path.Join(directory, RootCAFile)
			clientCertFile := path.Join(directory, ClientCertFile)
			clientPrivateKeyFile := path.Join(directory, ClientPrivateKeyFile)
			rootCA, err := os.ReadFile(rootCAFile)
			require.NoError(t, err)
			clientCert, err := os.ReadFile(clientCertFile)
			require.NoError(t, err)
			clientPrivateKey, err := os.ReadFile(clientPrivateKeyFile)
			require.NoError(t, err)
			require.Equal(t, server.fakeRootCertPEM, string(rootCA))
			require.Equal(t, server.fakeRootCertPEM, string(manager.RootCA()))
			require.Equal(t, server.fakeClientCert, string(clientCert))
			require.Equal(t, server.fakeClientCert, string(manager.Certificate()))
			require.Equal(t, server.fakeClientPrivateKey, string(clientPrivateKey))
			require.Equal(t, server.fakeClientPrivateKey, string(manager.PrivateKey()))
			require.NotNil(t, manager.TLSCertificate())
		})
	}
}

func TestManage_Refresh(t *testing.T) {
	t.Parallel()

	service := gwTesting.RandomString()

	server := runCertServer(t, service, 0, 0, 2)

	options := DefaultCertManagerOptions()
	manager := NewCertManager(
		hclog.Default().Named("certmanager"),
		Config{
			Addresses: []string{server.consulAddress},
			GRPCPort:  server.consulGRPCPort,
			TLS:       nil,
		},
		NewClient(server.consulHTTPClient),
		service,
		options,
	)
	manager.skipExtraFetch = true

	writes := int32(0)
	manager.writeCerts = func() error {
		numWrites := atomic.AddInt32(&writes, 1)
		if numWrites == 3 {
			require.Equal(t, server.fakeRootCertPEM, string(manager.ca))
			require.Equal(t, server.fakeClientCert, string(manager.certificate))
			require.Equal(t, server.fakeClientPrivateKey, string(manager.privateKey))
			close(manager.initializeSignal)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	managerErr := make(chan error, 2)
	go func() {
		if err := manager.Manage(ctx); err != nil {
			managerErr <- err
		}
	}()

	finished := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if err := manager.WaitForWrite(ctx); err != nil {
			managerErr <- err
		} else {
			close(finished)
		}
	}()

	select {
	case err := <-managerErr:
		require.NoError(t, err)
	case <-finished:
	}
}

func TestManage_WaitCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err := NewCertManager(
		hclog.Default().Named("certmanager"),
		Config{},
		nil,
		"",
		nil,
	).WaitForWrite(ctx)
	require.Error(t, err)
}

type testCAHandler struct {
	consulAddress  string
	consulHTTPPort int
	consulGRPCPort int

	consulHTTPClient *api.Client
	consulGRPCClient *pbconnectca.ConnectCAServiceClient

	ca                   *gwTesting.CertificateInfo
	fakeRootCertPEM      string
	fakeClientCert       string
	fakeClientPrivateKey string

	rotate chan struct{}

	mutex sync.RWMutex
}

func (c *testCAHandler) WatchRoots(request *pbconnectca.WatchRootsRequest, stream pbconnectca.ConnectCAService_WatchRootsServer) error {
	writeCertificate := func() error {
		fmt.Printf("writing certificate to channel")

		c.mutex.RLock()
		ca := c.fakeRootCertPEM
		c.mutex.RUnlock()

		if err := stream.Send(&pbconnectca.WatchRootsResponse{
			ActiveRootId: "test",
			Roots: []*pbconnectca.CARoot{{
				Id:       "test",
				RootCert: ca,
			}},
		}); err != nil {
			return err
		}
		return nil
	}

	// do initial write
	if err := writeCertificate(); err != nil {
		return err
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-c.rotate:
			if err := writeCertificate(); err != nil {
				return err
			}
		}
	}
}

func (c *testCAHandler) Rotate() {
	rootCA, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		IsCA: true,
	})
	if err != nil {
		panic(err)
	}

	c.mutex.Lock()
	c.ca = rootCA
	c.fakeRootCertPEM = string(rootCA.CertBytes)
	c.mutex.Unlock()

	c.rotate <- struct{}{}
}

func (c *testCAHandler) Sign(ctx context.Context, request *pbconnectca.SignRequest) (*pbconnectca.SignResponse, error) {
	return nil, fmt.Errorf("not yet implemented")
}

func runCertServer(t *testing.T, service string, leafFailures, rootFailures, expirations uint64) *testCAHandler {
	t.Helper()

	ca, _, clientCert := gwTesting.DefaultCertificates()
	expiredCert, err := gwTesting.GenerateSignedCertificate(gwTesting.GenerateCertificateOptions{
		CA:          ca,
		ServiceName: "client",
		Expiration:  time.Now().Add(-10 * time.Minute),
	})
	require.NoError(t, err)

	server := &testCAHandler{
		fakeRootCertPEM:      string(ca.CertBytes),
		fakeClientCert:       string(clientCert.CertBytes),
		fakeClientPrivateKey: string(clientCert.PrivateKeyBytes),
		rotate:               make(chan struct{}),
	}

	// Start the fake Consul HTTP server.
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leafPath := fmt.Sprintf("/v1/agent/connect/ca/leaf/%s", service)
		// rootPath := "/v1/agent/connect/ca/roots"
		meta := map[string]string{
			"X-Consul-Index": "1",
		}

		if r != nil && strings.HasPrefix(r.URL.Path, leafPath) && r.Method == "GET" {
			var expiration string
			clientCert := server.fakeClientCert
			clientPrivateKey := server.fakeClientPrivateKey
			if expirations == 0 {
				meta["X-Consul-Index"] = "2"
				expiration = time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
			} else {
				expiration = time.Now().Format(time.RFC3339)
				clientCert = string(expiredCert.CertBytes)
				clientPrivateKey = string(expiredCert.PrivateKeyBytes)
				expirations--
			}
			if leafFailures > 0 {
				leafFailures--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			leafCert, err := json.Marshal(map[string]interface{}{
				"CertPEM":       clientCert,
				"PrivateKeyPEM": clientPrivateKey,
				"ValidBefore":   expiration,
			})
			for k, v := range meta {
				w.Header().Add(k, v)
			}
			require.NoError(t, err)
			_, err = w.Write(leafCert)
			require.NoError(t, err)
			return
		}
		// if r != nil && strings.HasPrefix(r.URL.Path, rootPath) && r.Method == "GET" {
		// 	if rootFailures > 0 {
		// 		rootFailures--
		// 		w.WriteHeader(http.StatusInternalServerError)
		// 		return
		// 	}
		// 	rootCert, err := json.Marshal(map[string]interface{}{
		// 		"Roots": []map[string]interface{}{{
		// 			"RootCert": server.fakeRootCertPEM,
		// 			"Active":   true,
		// 		}},
		// 	})
		// 	require.NoError(t, err)
		// 	for k, v := range meta {
		// 		w.Header().Add(k, v)
		// 	}
		// 	_, err = w.Write(rootCert)
		// 	require.NoError(t, err)
		// 	return
		// }
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(httpServer.Close)

	serverURL, err := url.Parse(httpServer.URL)
	require.NoError(t, err)
	consulHTTPAddress := serverURL.Host

	clientConfig := &api.Config{Address: consulHTTPAddress}
	client, err := api.NewClient(clientConfig)
	require.NoError(t, err)
	server.consulHTTPClient = client

	// httptest.NewServer hardcodes 127.0.0.1, so this will be the same as for
	// the gRPC server, just on a different port
	consulHTTPAddressParts := strings.Split(consulHTTPAddress, ":")
	server.consulAddress = consulHTTPAddressParts[0]
	server.consulHTTPPort, err = strconv.Atoi(consulHTTPAddressParts[1])
	require.NoError(t, err)
	fmt.Printf("running Consul HTTP mock server at %s\n", consulHTTPAddress)

	// Start the fake Consul gRPC server
	grpcServer := grpc.NewServer()
	pbconnectca.RegisterConnectCAServiceServer(grpcServer, server)
	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	errEarlyTestTermination := errors.New("early termination")
	done := make(chan error, 1)
	go func() {
		defer func() {
			// Write an error to the channel, if the server canceled
			// successfully the err will be nil and the read will get that
			// first, this will only be read if we have some early exception
			// that calls runtime.Goexit prior to the server stopping
			done <- errEarlyTestTermination
		}()
		// Start gRPC mock server, send nil error if clean exit
		done <- grpcServer.Serve(grpcListener)
	}()
	server.consulGRPCPort, err = strconv.Atoi(strings.Split(grpcListener.Addr().String(), ":")[1])
	require.NoError(t, err)
	fmt.Printf("running Consul gRPC mock server at %s\n", grpcListener.Addr().String())

	return server
}

func TestRenderSDS(t *testing.T) {
	t.Parallel()

	expected := `
{
	"name": "sds-cluster",
	"connect_timeout": "5s",
	"type": "STRICT_DNS",
	"transport_socket": {
		"name": "tls",
		"typed_config": {
			"@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
			"common_tls_context": {
				"tls_certificate_sds_secret_configs": [
					{
						"name": "tls_sds",
						"sds_config": {
							"path": "/certs/tls-sds.json"
						}
					}
				],
				"validation_context_sds_secret_config": {
					"name": "validation_context_sds",
					"sds_config": {
						"path": "/certs/validation-context-sds.json"
					}
				}
			}
		}
	},
	"http2_protocol_options": {},
	"loadAssignment": {
		"clusterName": "sds-cluster",
		"endpoints": [
			{
				"lbEndpoints": [
					{
						"endpoint": {
							"address": {
								"socket_address": {
									"address": "localhost",
									"port_value": 9090
								}
							}
						}
					}
				]
			}
		]
	}
}
`
	directory, err := os.MkdirTemp("", "consul-api-gateway-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	options := DefaultCertManagerOptions()
	options.Directory = "/certs"
	manager := NewCertManager(
		hclog.Default().Named("certmanager"),
		Config{},
		nil,
		gwTesting.RandomString(),
		options,
	)
	manager.configDirectory = directory

	config, err := manager.RenderSDSConfig()
	require.NoError(t, err)
	var buffer bytes.Buffer
	err = json.Indent(&buffer, []byte(config), "", "  ")
	require.NoError(t, err)

	require.JSONEq(t, expected, buffer.String())
}
