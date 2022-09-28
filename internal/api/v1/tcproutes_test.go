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

func TestServer_ListTCPRoutes(t *testing.T) {
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
			resp, err := http.Get(testServer.URL + "/tcp-routes")
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}

func TestServer_GetNamespacedTCPRoute(t *testing.T) {
	s := NewServer("", nil, "foo", "", testConsul(t, false), hclog.NewNullLogger())

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	validator := NewMockValidator(ctrl)

	s := NewServer("", validator, "foo", "", testConsul(t, false), hclog.NewNullLogger())

	testServer := httptest.NewServer(s)
	defer testServer.Close()

	tests := []struct {
		name                   string
		route                  *TCPRoute
		runtimeValidationError string
		wantStatusCode         int
		wantError              string
	}{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// set up expectations
			if tt.runtimeValidationError != "" {
				validator.EXPECT().ValidateTCPRoute(gomock.Any(), gomock.Any()).Return(errors.New(tt.runtimeValidationError))
			} else if tt.wantError == "" {
				validator.EXPECT().ValidateTCPRoute(gomock.Any(), gomock.Any()).Return(nil)
			}

			data, err := json.Marshal(tt.route)
			require.NoError(t, err)
			resp, err := http.Post(testServer.URL+"/tcp-routes", "application/json", bytes.NewBuffer(data))
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

func TestServer_DeleteNamespacedTCPRoute(t *testing.T) {
	s := NewServer("", nil, "foo", "", testConsul(t, false), hclog.NewNullLogger())

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
