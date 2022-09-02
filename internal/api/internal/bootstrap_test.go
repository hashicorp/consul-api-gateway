package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServer_Bootstrap(t *testing.T) {
	s, err := NewServer("", nil, hclog.NewNullLogger())
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
			resp, err := http.Get(testServer.URL + "/bootstrap")
			require.NoError(t, err)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}
