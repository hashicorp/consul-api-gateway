// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
)

func TestRegister(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, test := range []struct {
		name       string
		host       string
		failures   uint64
		maxRetries uint64
		fail       bool
	}{{
		name: "basic-test",
		host: "localhost",
	}, {
		name:       "retry-success",
		host:       "localhost",
		failures:   3,
		maxRetries: 3,
	}, {
		name:       "retry-failure",
		host:       "localhost",
		failures:   3,
		maxRetries: 2,
		fail:       true,
	}} {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.New().String()
			service := gwTesting.RandomString()
			namespace := gwTesting.RandomString()

			maxRetries := defaultMaxRetries
			if test.maxRetries > 0 {
				maxRetries = test.maxRetries
			}

			server := runRegistryServer(t, test.failures, id)
			registry := NewServiceRegistry(hclog.NewNullLogger(), NewTestClient(server.consul), service, namespace, "", test.host).WithRetries(maxRetries)

			registry.backoffInterval = 0
			registry.id = id

			err := registry.RegisterGateway(ctx, false)
			if test.fail {
				require.Error(t, err)
				return
			}
			defer require.NoError(t, registry.Deregister(context.Background()))

			require.NoError(t, err)
			require.Equal(t, id, registry.ID())
			require.Equal(t, id, server.lastRegistrationRequest.ID)
			require.Equal(t, service, server.lastRegistrationRequest.Service.Service)
			require.Equal(t, namespace, server.lastRegistrationRequest.Service.Namespace)
			require.Equal(t, test.host, server.lastRegistrationRequest.Service.Address)
			require.Len(t, server.lastRegistrationRequest.Checks, 1)
			require.Equal(t, fmt.Sprintf("%s:20000", test.host), server.lastRegistrationRequest.Checks[0].Definition.TCP)
		})
	}
}

func TestDeregister(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		failures   uint64
		maxRetries uint64
		fail       bool
	}{{
		name: "basic-test",
	}, {
		name:       "retry-success",
		failures:   3,
		maxRetries: 3,
	}, {
		name:       "retry-fail",
		failures:   3,
		maxRetries: 2,
		fail:       true,
	}} {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.New().String()
			service := gwTesting.RandomString()

			maxRetries := defaultMaxRetries
			if test.maxRetries > 0 {
				maxRetries = test.maxRetries
			}

			server := runRegistryServer(t, test.failures, id)
			registry := NewServiceRegistry(hclog.NewNullLogger(), NewTestClient(server.consul), service, "", "", "").WithRetries(maxRetries)
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

	lastRegistrationRequest api.CatalogRegistration
	deregistered            bool
}

func runRegistryServer(t *testing.T, failures uint64, id string) *registryServer {
	t.Helper()

	server := &registryServer{}

	registerPath := "/v1/catalog/register"
	deregisterPath := "/v1/catalog/deregister"

	// Start the fake Consul server.
	consulServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failures > 0 {
			failures--
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r != nil && r.URL.Path == registerPath && r.Method == "PUT" {
			body, err := io.ReadAll(r.Body)
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
