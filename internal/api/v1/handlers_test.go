package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

const testToken = "6d1b28fc-3ccf-4a26-ab19-1ba1c103ade3"

func testConsul(t *testing.T, aclEnabled bool) *api.Client {
	t.Helper()

	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.ACL.Enabled = aclEnabled
		c.ACL.Tokens.InitialManagement = testToken
		c.Peering = nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = consulSrv.Stop()
	})
	consulSrv.WaitForLeader(t)

	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	cfg.Token = testToken
	consul, err := api.NewClient(cfg)
	require.NoError(t, err)
	return consul
}

func TestServer_ListGateways(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name           string
		wantStatusCode int
	}{
		{
			name:           "stub",
			wantStatusCode: http.StatusNotImplemented,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(testServer.URL + "/gateways")
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}

func TestServer_GetGateway(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name             string
		gatewayNamespace string
		gatewayName      string
		wantStatusCode   int
	}{
		{
			name:             "stub",
			gatewayNamespace: "a",
			gatewayName:      "b",
			wantStatusCode:   http.StatusNotImplemented,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(testServer.URL + "/gateways/" + tt.gatewayNamespace + "/" + tt.gatewayName)
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}

func TestServer_AddGateway(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name           string
		gateway        *Gateway
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "validate-listeners",
			gateway:        &Gateway{Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "listeners",
		},
		{
			name:           "validate-listeners-protocol",
			gateway:        &Gateway{Listeners: []Listener{{}}, Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "listeners.0.protocol",
		},
		{
			name: "validate-name",
			gateway: &Gateway{Listeners: []Listener{{
				Protocol: "http",
			}}, Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "name",
		},
		{
			name: "validate-namespace",
			gateway: &Gateway{Listeners: []Listener{{
				Protocol: "http",
			}}, Name: "a"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "namespace",
		},
		{
			name: "pass-validation",
			gateway: &Gateway{Listeners: []Listener{{
				Protocol: "http",
			}}, Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusCreated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.gateway)
			require.NoError(t, err)
			resp, err := http.Post(testServer.URL+"/gateways", "application/json", bytes.NewBuffer(data))
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
			if tt.wantError != "" {
				body, err := ioutil.ReadAll(resp.Body)
				require.NoError(t, err)
				resp.Body.Close()
				require.Contains(t, string(body), fmt.Sprintf("\"%s:", tt.wantError))
			}
		})
	}
}

func TestServer_DeleteGateway(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name             string
		gatewayNamespace string
		gatewayName      string
		wantStatusCode   int
	}{
		{
			name:             "non-existent",
			gatewayNamespace: "a",
			gatewayName:      "b",
			wantStatusCode:   http.StatusAccepted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("DELETE", testServer.URL+"/gateways/"+tt.gatewayNamespace+"/"+tt.gatewayName, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}
