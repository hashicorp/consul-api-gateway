package apiinternal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestClient_Bootstrap(t *testing.T) {
	s := NewServer("", BootstrapConfiguration{}, nil, hclog.NewNullLogger())

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
				Server:  testServer.URL,
				BaseURL: "/",
			})
			require.NoError(t, err)
			_, err = client.Bootstrap(context.Background())
			require.Error(t, err)
		})
	}
}
