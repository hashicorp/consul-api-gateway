package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/metrics"
	polarTesting "github.com/hashicorp/polar/internal/testing"
)

var (
	generate bool
)

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

func TestManage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		leafFailures uint64
		rootFailures uint64
		maxRetries   uint64
		fail         bool
	}{{
		name: "test-basic",
	}, {
		name:         "test-leaf-retries",
		leafFailures: 3,
		maxRetries:   3,
	}, {
		name:         "test-root-retries",
		rootFailures: 3,
		maxRetries:   3,
	}, {
		name:         "test-mix-retries",
		leafFailures: 2,
		rootFailures: 1,
		maxRetries:   3,
	}, {
		name:         "test-leaf-retry-failures",
		leafFailures: 3,
		maxRetries:   2,
		fail:         true,
	}, {
		name:         "test-root-retry-failures",
		rootFailures: 3,
		maxRetries:   2,
		fail:         true,
	}, {
		name:         "test-mix-retry-failures",
		leafFailures: 2,
		rootFailures: 3,
		maxRetries:   2,
		fail:         true,
	}} {
		t.Run(test.name, func(t *testing.T) {
			directory, err := os.MkdirTemp("", randomString())
			require.NoError(t, err)
			defer os.RemoveAll(directory)
			service := randomString()

			server := runCertServer(t, test.leafFailures, test.rootFailures, service, 0)

			options := DefaultCertManagerOptions()
			options.Directory = directory
			options.Tries = test.maxRetries

			manager := NewCertManager(hclog.NewNullLogger(), metrics.Registry.Consul, server.consul, service, options)
			manager.backoffInterval = 0

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
				if test.fail {
					require.Error(t, err)
					return
				}
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

	service := randomString()

	server := runCertServer(t, 0, 0, service, 2)

	options := DefaultCertManagerOptions()
	manager := NewCertManager(hclog.NewNullLogger(), metrics.Registry.Consul, server.consul, service, options)
	manager.backoffInterval = 0

	writes := int32(0)
	manager.writeCerts = func(root *api.CARoot, client *api.LeafCert) error {
		numWrites := atomic.AddInt32(&writes, 1)
		if numWrites == 3 {
			close(manager.initializeSignal)
		}
		require.Equal(t, server.fakeRootCertPEM, root.RootCertPEM)
		require.Equal(t, server.fakeClientCert, client.CertPEM)
		require.Equal(t, server.fakeClientPrivateKey, client.PrivateKeyPEM)
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

	err := NewCertManager(hclog.NewNullLogger(), metrics.Registry.Consul, nil, "", nil).WaitForWrite(ctx)
	require.Error(t, err)
}

type certServer struct {
	consul *api.Client

	fakeRootCertPEM      string
	fakeClientCert       string
	fakeClientPrivateKey string
}

func runCertServer(t *testing.T, leafFailures, rootFailures uint64, service string, expirations int32) *certServer {
	t.Helper()

	ca, _, clientCert := polarTesting.DefaultCertificates()
	server := &certServer{
		fakeRootCertPEM:      string(ca.CertBytes),
		fakeClientCert:       string(clientCert.CertBytes),
		fakeClientPrivateKey: string(clientCert.PrivateKeyBytes),
	}

	// Start the fake Consul server.
	consulServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leafPath := fmt.Sprintf("/v1/agent/connect/ca/leaf/%s", service)
		rootPath := "/v1/agent/connect/ca/roots"
		if r != nil && r.URL.Path == leafPath && r.Method == "GET" {
			var expiration string
			if expirations == 0 {
				expiration = time.Now().Add(10 * time.Minute).Format(time.RFC3339)
			} else {
				expiration = time.Now().Format(time.RFC3339)
				expirations--
			}
			if leafFailures > 0 {
				leafFailures--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			leafCert, err := json.Marshal(map[string]interface{}{
				"CertPEM":       server.fakeClientCert,
				"PrivateKeyPEM": server.fakeClientPrivateKey,
				"ValidBefore":   expiration,
			})
			require.NoError(t, err)
			w.Write([]byte(leafCert))
			return
		}
		if r != nil && r.URL.Path == rootPath && r.Method == "GET" {
			if rootFailures > 0 {
				rootFailures--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			rootCert, err := json.Marshal(map[string]interface{}{
				"Roots": []map[string]interface{}{{
					"RootCert": server.fakeRootCertPEM,
					"Active":   true,
				}},
			})
			require.NoError(t, err)
			w.Write([]byte(rootCert))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consulServer.Close)

	serverURL, err := url.Parse(consulServer.URL)
	require.NoError(t, err)
	clientConfig := &api.Config{Address: serverURL.String()}
	client, err := api.NewClient(clientConfig)
	require.NoError(t, err)

	server.consul = client
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
	directory, err := os.MkdirTemp("", "polar-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	options := DefaultCertManagerOptions()
	manager := NewCertManager(hclog.NewNullLogger(), metrics.Registry.Consul, nil, randomString(), options)
	manager.configDirectory = directory

	config, err := manager.RenderSDSConfig()
	require.NoError(t, err)
	var buffer bytes.Buffer
	err = json.Indent(&buffer, []byte(config), "", "  ")
	require.NoError(t, err)

	require.JSONEq(t, expected, buffer.String())
}
