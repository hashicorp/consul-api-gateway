package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
)

func TestRunExecLoginError(t *testing.T) {
	t.Parallel()

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{
		loginFail: true,
	})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "error logging into consul")
}

func TestRunExecLoginSuccessRegistrationFail(t *testing.T) {
	t.Parallel()

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{
		registerFail: true,
	})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "error registering service")
}

func TestRunExecDeregisterFail(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{
		deregisterFail: true,
		// force early deregistration
		leafCertFail: true,
	})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			CertificateDirectory: directory,
			BootstrapFile:        path.Join(directory, "boostrap.json"),
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "error deregistering service")
}

func TestRunExecCertFail(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{
		leafCertFail: true,
	})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			CertificateDirectory: directory,
			BootstrapFile:        path.Join(directory, "boostrap.json"),
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "timeout waiting for certs to be written")
}

func TestRunExecRootCertFail(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{
		rootCertFail: true,
	})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			CertificateDirectory: directory,
			BootstrapFile:        path.Join(directory, "boostrap.json"),
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "timeout waiting for certs to be written")
}

func TestRunExecSDSRenderFail(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			// render certs to a non-existent directory
			CertificateDirectory: directory,
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "error rendering SDS configuration files")
}

func TestRunExecBootstrapRenderFail(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			CertificateDirectory: directory,
			// render boostrap file to a file in a non-existent directory
			BootstrapFile: path.Join("nonexistent", "path"),
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "error rendering Envoy configuration file")
}

func TestRunExecEnvoyExecError(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{})
	require.Equal(t, 1, RunExec(ExecConfig{
		Context:      context.Background(),
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			CertificateDirectory: directory,
			BootstrapFile:        path.Join(directory, "boostrap.json"),
			Binary:               "thisisnotabinary",
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), "unexpected error")
	require.Contains(t, buffer.String(), "thisisnotabinary")
}

func TestRunExecShutdown(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	output := gwTesting.RandomString()
	require.Equal(t, 0, RunExec(ExecConfig{
		Context:      ctx,
		Logger:       logger,
		ConsulClient: consul.client,
		ConsulConfig: *consul.config,
		AuthConfig: AuthConfig{
			Method: "nonexistent",
			Token:  "token",
		},
		GatewayConfig: GatewayConfig{
			Name: "test",
		},
		EnvoyConfig: EnvoyConfig{
			CertificateDirectory: directory,
			BootstrapFile:        path.Join(directory, "boostrap.json"),
			Binary:               "echo",
			ExtraArgs:            []string{output},
			Output:               &buffer,
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), output)
	require.Contains(t, buffer.String(), "shutting down")
}

type mockConsulOptions struct {
	loginFail      bool
	registerFail   bool
	deregisterFail bool
	leafCertFail   bool
	rootCertFail   bool
}

type mockConsulServer struct {
	client *api.Client
	config *api.Config

	token            string
	rootCertPEM      string
	clientCert       string
	clientPrivateKey string
}

func runMockConsulServer(t *testing.T, opts mockConsulOptions) *mockConsulServer {
	t.Helper()

	ca, _, clientCert := gwTesting.DefaultCertificates()
	server := &mockConsulServer{
		token:            gwTesting.RandomString(),
		rootCertPEM:      string(ca.CertBytes),
		clientCert:       string(clientCert.CertBytes),
		clientPrivateKey: string(clientCert.PrivateKeyBytes),
	}

	loginPath := "/v1/acl/login"
	registerPath := "/v1/agent/service/register"
	deregisterPath := "/v1/agent/service/deregister"
	leafPath := "/v1/agent/connect/ca/leaf"
	rootPath := "/v1/agent/connect/ca/roots"

	// Start the fake Consul server.
	consulServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r != nil && r.URL.Path == loginPath && r.Method == "POST" {
			if opts.loginFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err := w.Write([]byte(fmt.Sprintf(`{"SecretID": "%s"}`, server.token)))
			require.NoError(t, err)
			return
		}
		if r != nil && r.URL.Path == registerPath && r.Method == "PUT" {
			if opts.registerFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		}
		if r != nil && strings.HasPrefix(r.URL.Path, deregisterPath) && r.Method == "PUT" {
			if opts.deregisterFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		}
		if r != nil && strings.HasPrefix(r.URL.Path, leafPath) && r.Method == "GET" {
			if opts.leafCertFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			leafCert, err := json.Marshal(map[string]interface{}{
				"CertPEM":       server.clientCert,
				"PrivateKeyPEM": server.clientPrivateKey,
				"ValidBefore":   time.Now().Add(10 * time.Hour),
			})
			require.NoError(t, err)
			_, err = w.Write([]byte(leafCert))
			require.NoError(t, err)
			return
		}
		if r != nil && r.URL.Path == rootPath && r.Method == "GET" {
			if opts.rootCertFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			rootCert, err := json.Marshal(map[string]interface{}{
				"Roots": []map[string]interface{}{{
					"RootCert": server.rootCertPEM,
					"Active":   true,
				}},
			})
			require.NoError(t, err)
			_, err = w.Write([]byte(rootCert))
			require.NoError(t, err)
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

	server.client = client
	server.config = clientConfig
	return server
}
