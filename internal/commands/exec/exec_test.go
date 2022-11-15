package exec

import (
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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/proto-public/pbconnectca"
	"github.com/hashicorp/go-hclog"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
			XDSAddress:           consul.address,
			XDSPort:              consul.grpcPort,
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
			XDSAddress:           consul.address,
			XDSPort:              consul.grpcPort,
		},
		isTest: true,
	}))
	require.Contains(t, buffer.String(), output)
	require.Contains(t, buffer.String(), "shutting down")
}

// TestRunExecShutdownACLs test that if ACLs are enabled we logout and report error if this fails.
func TestRunExecShutdownACLs(t *testing.T) {
	t.Parallel()

	directory, err := os.MkdirTemp("", "exec-test")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	consul := runMockConsulServer(t, mockConsulOptions{
		loginFail:  false,
		logoutFail: true,
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	output := gwTesting.RandomString()
	require.Equal(t, 1, RunExec(ExecConfig{
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
			XDSAddress:           consul.address,
			XDSPort:              consul.grpcPort,
		},
		isTest: true,
	}))

	require.Contains(t, buffer.String(), output)
	require.Contains(t, buffer.String(), "error deleting acl token")
}

type mockConsulOptions struct {
	loginFail      bool
	logoutFail     bool
	registerFail   bool
	deregisterFail bool
	leafCertFail   bool
	rootCertFail   bool
}

type mockConsulServer struct {
	client *api.Client
	config *api.Config

	address  string
	httpPort int
	grpcPort int

	token            string
	rootCertPEM      string
	clientCert       string
	clientPrivateKey string

	mutex sync.RWMutex

	opts mockConsulOptions
}

func (c *mockConsulServer) WatchRoots(request *pbconnectca.WatchRootsRequest, stream pbconnectca.ConnectCAService_WatchRootsServer) error {
	writeCertificate := func() error {
		fmt.Printf("writing certificate to channel")

		if c.opts.rootCertFail {
			return status.Error(codes.Internal, "root watch failure")
		}

		c.mutex.RLock()
		ca := c.rootCertPEM
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
			// case <-c.rotate:
			// 	if err := writeCertificate(); err != nil {
			// 		return err
			// 	}
		}
	}
}

func (c *mockConsulServer) Sign(ctx context.Context, request *pbconnectca.SignRequest) (*pbconnectca.SignResponse, error) {
	return nil, fmt.Errorf("not yet implemented")
}

func runMockConsulServer(t *testing.T, opts mockConsulOptions) *mockConsulServer {
	t.Helper()

	ca, _, clientCert := gwTesting.DefaultCertificates()
	server := &mockConsulServer{
		token:            gwTesting.RandomString(),
		rootCertPEM:      string(ca.CertBytes),
		clientCert:       string(clientCert.CertBytes),
		clientPrivateKey: string(clientCert.PrivateKeyBytes),
		opts:             opts,
	}

	loginPath := "/v1/acl/login"
	logoutPath := "/v1/acl/logout"
	registerPath := "/v1/agent/service/register"
	deregisterPath := "/v1/agent/service/deregister"
	leafPath := "/v1/agent/connect/ca/leaf"

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
		if r != nil && r.URL.Path == logoutPath && r.Method == "POST" {
			if opts.logoutFail {
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
			_, err = w.Write(leafCert)
			require.NoError(t, err)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consulServer.Close)

	serverURL, err := url.Parse(consulServer.URL)
	require.NoError(t, err)
	consulHTTPAddress := serverURL.Host

	clientConfig := &api.Config{Address: serverURL.String()}
	client, err := api.NewClient(clientConfig)
	require.NoError(t, err)
	server.client = client
	server.config = clientConfig

	// httptest.NewServer hardcodes 127.0.0.1, so this will be the same as for
	// the gRPC server, just on a different port
	consulHTTPAddressParts := strings.Split(consulHTTPAddress, ":")
	server.address = consulHTTPAddressParts[0]
	server.httpPort, err = strconv.Atoi(consulHTTPAddressParts[1])
	require.NoError(t, err)
	fmt.Printf("running Consul HTTP mock server at %s\n", serverURL.String())

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
	server.grpcPort, err = strconv.Atoi(strings.Split(grpcListener.Addr().String(), ":")[1])
	require.NoError(t, err)
	fmt.Printf("running Consul gRPC mock server at %s\n", grpcListener.Addr().String())

	return server
}
