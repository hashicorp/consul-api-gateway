package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServer_FindGateways(t *testing.T) {
	s := NewServer("", hclog.NewNullLogger())
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
			require.Equal(t, tt.wantStatusCode, resp.StatusCode)
		})
	}
}
