package consul

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestManage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		leafFailures uint64
		rootFailures uint64
		maxRetries   uint64
		writes       int
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
	}, {
		name:   "test-refresh-cert",
		writes: 3,
	}} {
		t.Run(test.name, func(t *testing.T) {
			directory, err := os.MkdirTemp("", randomString())
			require.NoError(t, err)
			defer os.RemoveAll(directory)

			service := randomString()

			expirations := test.writes - 1
			server := runCertServer(t, test.leafFailures, test.rootFailures, service, expirations)

			options := DefaultCertManagerOptions()
			options.Directory = directory
			options.Tries = test.maxRetries
			if test.writes > 0 {
				options.SignalOnNWrites = test.writes
			}

			manager := NewCertManager(hclog.NewNullLogger(), server.consul, service, options)
			manager.backoffInterval = 0

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			managerErr := make(chan error, 1)
			go func() {
				if err := manager.Manage(ctx); err != nil {
					managerErr <- err
				}
			}()

			initialized := make(chan struct{})
			go func() {
				manager.WaitForWrite(context.Background())
				close(initialized)
			}()

			select {
			case err := <-managerErr:
				if test.fail {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
			case <-initialized:
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
			require.Equal(t, server.fakeClientCert, string(clientCert))
			require.Equal(t, server.fakeClientPrivateKey, string(clientPrivateKey))
		})
	}
}

func TestManage_WaitCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err := NewCertManager(hclog.NewNullLogger(), nil, "", nil).WaitForWrite(ctx)
	require.Error(t, err)
}

type certServer struct {
	consul *api.Client

	fakeRootCertPEM      string
	fakeClientCert       string
	fakeClientPrivateKey string
}

func runCertServer(t *testing.T, leafFailures, rootFailures uint64, service string, expirations int) *certServer {
	t.Helper()

	server := &certServer{
		fakeRootCertPEM:      randomString(),
		fakeClientCert:       randomString(),
		fakeClientPrivateKey: randomString(),
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
			leafCert := fmt.Sprintf(`{ "CertPEM": "%s","PrivateKeyPEM": "%s","ValidBefore": "%s" }`, server.fakeClientCert, server.fakeClientPrivateKey, expiration)
			w.Write([]byte(leafCert))
			return
		}
		if r != nil && r.URL.Path == rootPath && r.Method == "GET" {
			if rootFailures > 0 {
				rootFailures--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			rootCert := fmt.Sprintf(`{"Roots":[{"RootCert": "%s","Active": true}]}`, server.fakeRootCertPEM)
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
