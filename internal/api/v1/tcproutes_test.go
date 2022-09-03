package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServer_ListTCPRoutes(t *testing.T) {
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
			resp, err := http.Get(testServer.URL + "/tcp-routes")
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}

func TestServer_GetNamespacedTCPRoute(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name           string
		routeNamespace string
		routeName      string
		wantStatusCode int
	}{
		{
			name:           "stub",
			routeNamespace: "a",
			routeName:      "b",
			wantStatusCode: http.StatusNotImplemented,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(testServer.URL + "/namespaces/" + tt.routeNamespace + "/tcp-routes/" + tt.routeName)
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}

func TestServer_CreateTCPRoute(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name           string
		route          *TCPRoute
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "validate-gateways",
			route:          &TCPRoute{Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "gateways",
		},
		{
			name:           "validate-gateways-length",
			route:          &TCPRoute{Gateways: []GatewayReference{}, Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "gateways",
		},
		{
			name: "validate-gateways-namespace",
			route: &TCPRoute{Gateways: []GatewayReference{
				{Name: "a"},
			}, Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "gateways.0.namespace",
		},
		{
			name: "validate-gateways-name",
			route: &TCPRoute{Gateways: []GatewayReference{
				{Namespace: "b"},
			}, Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "gateways.0.name",
		},
		{
			name: "validate-name",
			route: &TCPRoute{Gateways: []GatewayReference{
				{Name: "a", Namespace: "b"},
			}, Namespace: "b"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "name",
		},
		{
			name: "validate-namespace",
			route: &TCPRoute{Gateways: []GatewayReference{
				{Name: "a", Namespace: "b"},
			}, Name: "a"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "namespace",
		},
		{
			name: "pass-validation",
			route: &TCPRoute{Gateways: []GatewayReference{
				{Name: "a", Namespace: "b"},
			}, Name: "a", Namespace: "b"},
			wantStatusCode: http.StatusCreated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.route)
			require.NoError(t, err)
			resp, err := http.Post(testServer.URL+"/tcp-routes", "application/json", bytes.NewBuffer(data))
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

func TestServer_DeleteNamespacedTCPRoute(t *testing.T) {
	s, err := NewServer("", testConsul(t, false), hclog.NewNullLogger())
	require.NoError(t, err)

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name           string
		routeNamespace string
		routeName      string
		wantStatusCode int
	}{
		{
			name:           "non-existent",
			routeNamespace: "a",
			routeName:      "b",
			wantStatusCode: http.StatusAccepted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("DELETE", testServer.URL+"/namespaces/"+tt.routeNamespace+"/tcp-routes/"+tt.routeName, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}
