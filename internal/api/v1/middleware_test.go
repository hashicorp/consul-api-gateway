package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServer_consulTokenMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		aclEnabled bool
		token      string
		authorized bool
	}{
		{
			name:       "acl disabled",
			aclEnabled: false,
			authorized: true,
		},
		{
			name:       "acl enabled and authorized",
			aclEnabled: true,
			token:      testToken,
			authorized: true},
		{
			name:       "acl enabled and unauthorized",
			aclEnabled: true,
			token:      "fake-token",
			authorized: false},
		{
			name:       "acl enabled and no token",
			aclEnabled: true,
			authorized: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewServer("", testConsul(t, tt.aclEnabled), hclog.Default())
			testServer := httptest.NewServer(s)
			defer testServer.Close()
			req, err := http.NewRequest(http.MethodGet, testServer.URL+"/fake", nil)
			require.NoError(t, err)
			req.Header.Set(consulTokenHeader, tt.token)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			if tt.authorized {
				require.Equal(t, http.StatusNotFound, resp.StatusCode)
			} else {
				require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			}

		})
	}
}
