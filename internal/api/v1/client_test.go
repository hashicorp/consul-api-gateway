package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestClient_ListGateways(t *testing.T) {
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
			client, err := CreateClient(ClientConfig{
				Server: testServer.URL,
				Base:   "/",
			})
			require.NoError(t, err)
			_, err = client.ListGateways(context.Background())
			require.Error(t, err)
		})
	}
}
