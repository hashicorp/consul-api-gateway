package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

func TestRegister(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		host        string
		failures    uint64
		maxAttempts uint64
		fail        bool
	}{{
		name: "basic-test",
		host: "localhost",
	}, {
		name:        "test-retries",
		host:        "localhost",
		failures:    3,
		maxAttempts: 3,
	}, {
		name:        "test-retries-fail",
		host:        "localhost",
		failures:    3,
		maxAttempts: 2,
		fail:        true,
	}} {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.New().String()
			service := gwTesting.RandomString()
			namespace := gwTesting.RandomString()

			maxAttempts := defaultMaxAttempts
			if test.maxAttempts > 0 {
				maxAttempts = test.maxAttempts
			}

			server := runRegistryServer(t, test.failures, id)
			registry := NewServiceRegistry(hclog.NewNullLogger(), server.consul, service, namespace, test.host).WithTries(maxAttempts)
			registry.backoffInterval = 0
			registry.id = id

			err := registry.Register(context.Background())
			if test.fail {
				require.Error(t, err)
				return
			}
			defer require.NoError(t, registry.Deregister(context.Background()))

			require.NoError(t, err)
			require.Equal(t, id, registry.ID())
			require.Equal(t, id, server.lastRegistrationRequest.ID)
			require.Equal(t, service, server.lastRegistrationRequest.Name)
			require.Equal(t, namespace, server.lastRegistrationRequest.Namespace)
			require.Equal(t, test.host, server.lastRegistrationRequest.Address)
			require.Len(t, server.lastRegistrationRequest.Checks, 1)
			require.Equal(t, fmt.Sprintf("%s:20000", test.host), server.lastRegistrationRequest.Checks[0].TCP)
		})
	}
}

func TestDeregister(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		failures    uint64
		maxAttempts uint64
		fail        bool
	}{{
		name: "basic-test",
	}, {
		name:        "test-retries",
		failures:    3,
		maxAttempts: 3,
	}, {
		name:        "test-retries-fail",
		failures:    3,
		maxAttempts: 2,
		fail:        true,
	}} {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.New().String()
			service := gwTesting.RandomString()

			maxAttempts := defaultMaxAttempts
			if test.maxAttempts > 0 {
				maxAttempts = test.maxAttempts
			}

			server := runRegistryServer(t, test.failures, id)
			registry := NewServiceRegistry(hclog.NewNullLogger(), server.consul, service, "", "").WithTries(maxAttempts)
			registry.backoffInterval = 0
			registry.id = id
			err := registry.Deregister(context.Background())
			if test.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, server.deregistered)
		})
	}
}

type registryServer struct {
	consul *api.Client

	lastRegistrationRequest api.AgentServiceRegistration
	deregistered            bool
}

func runRegistryServer(t *testing.T, failures uint64, id string) *registryServer {
	t.Helper()

	server := &registryServer{}

	registerPath := "/v1/agent/service/register"
	deregisterPath := fmt.Sprintf("/v1/agent/service/deregister/%s", id)

	// Start the fake Consul server.
	consulServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failures > 0 {
			failures--
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r != nil && r.URL.Path == registerPath && r.Method == "PUT" {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Errorf("error reading request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if err := json.Unmarshal(body, &server.lastRegistrationRequest); err != nil {
				t.Errorf("error unmarshaling request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		}
		if r != nil && r.URL.Path == deregisterPath && r.Method == "PUT" {
			server.deregistered = true
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
