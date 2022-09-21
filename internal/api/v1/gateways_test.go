package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServer_ListGateways(t *testing.T) {
	s := NewServer("", nil, "foo", "", testConsul(t, false), hclog.NewNullLogger())

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

func TestServer_GetNamespacedGateway(t *testing.T) {
	s := NewServer("", nil, "foo", "", testConsul(t, false), hclog.NewNullLogger())

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
			resp, err := http.Get(testServer.URL + "/namespaces/" + tt.gatewayNamespace + "/gateways/" + tt.gatewayName)
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}

func TestServer_CreateGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	validator := NewMockValidator(ctrl)

	s := NewServer("", validator, "foo", "", testConsul(t, false), hclog.NewNullLogger())

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name                   string
		gateway                *Gateway
		runtimeValidationError string
		wantStatusCode         int
		wantError              string
	}{
		{
			name:           "validate-listeners",
			gateway:        &Gateway{Name: "a"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "listeners",
		},
		{
			name: "validate-listeners-port",
			gateway: &Gateway{Listeners: []Listener{{
				Protocol: ListenerProtocolHttp,
			}}, Name: "a"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "listeners.0.port",
		},
		{
			name: "validate-listeners-protocol",
			gateway: &Gateway{Listeners: []Listener{{
				Port: 1,
			}}, Name: "a"},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "listeners.0.protocol",
		},
		{
			name: "validate-name",
			gateway: &Gateway{Listeners: []Listener{{
				Port:     1,
				Protocol: "http",
			}}},
			wantStatusCode: http.StatusBadRequest,
			wantError:      "name",
		},
		{
			name: "pass-validation",
			gateway: &Gateway{Listeners: []Listener{{
				Port:     1,
				Protocol: "http",
			}}, Name: "a"},
			wantStatusCode: http.StatusCreated,
		},
		{
			name: "runtime-validation-error",
			gateway: &Gateway{Listeners: []Listener{{
				Port:     1,
				Protocol: "http",
			}}, Name: "a"},
			wantStatusCode:         http.StatusBadRequest,
			runtimeValidationError: "foobar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// set up expectations
			if tt.runtimeValidationError != "" {
				validator.EXPECT().ValidateGateway(gomock.Any(), gomock.Any()).Return(errors.New(tt.runtimeValidationError))
			} else if tt.wantError == "" {
				validator.EXPECT().ValidateGateway(gomock.Any(), gomock.Any()).Return(nil)
			}

			data, err := json.Marshal(tt.gateway)
			require.NoError(t, err)
			resp, err := http.Post(testServer.URL+"/gateways", "application/json", bytes.NewBuffer(data))
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			resp.Body.Close()

			if tt.wantError != "" {
				require.Contains(t, string(body), fmt.Sprintf("\"%s:", tt.wantError))
			} else if tt.runtimeValidationError != "" {
				require.Contains(t, string(body), fmt.Sprintf("\"%s\"", tt.runtimeValidationError))
			}
		})
	}
}

func TestServer_DeleteNamespacedGateway(t *testing.T) {
	s := NewServer("", nil, "foo", "", testConsul(t, false), hclog.NewNullLogger())

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
			req, err := http.NewRequest("DELETE", testServer.URL+"/namespaces/"+tt.gatewayNamespace+"/gateways/"+tt.gatewayName, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}
