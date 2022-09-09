package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServer_consulTokenMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		aclEnabled  bool
		token       string
		createToken bool
		authorized  bool
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
			authorized: true,
		},
		{
			name:        "acl enabled and unauthorized token",
			aclEnabled:  true,
			createToken: true,
			authorized:  false,
		},
		{
			name:       "acl enabled and invalid token",
			aclEnabled: true,
			token:      "fake-token",
			authorized: false,
		},
		{
			name:       "acl enabled and no token",
			aclEnabled: true,
			authorized: false,
		},
	}
	for _, tt := range tests {
		// need to shadow this for parallel run
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			consul := testConsul(t, tt.aclEnabled)
			s := NewServer("", "foo", "", consul, hclog.Default())

			token := tt.token
			if tt.createToken {
				aclToken, _, err := consul.ACL().TokenCreate(&api.ACLToken{}, nil)
				require.NoError(t, err)
				token = aclToken.SecretID
			}

			testServer := httptest.NewServer(s)
			defer testServer.Close()

			req, err := http.NewRequest(http.MethodGet, testServer.URL+"/fake", nil)
			require.NoError(t, err)
			req.Header.Set(consulTokenHeader, token)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			if tt.authorized {
				require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			} else {
				require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			}
		})
	}
}
