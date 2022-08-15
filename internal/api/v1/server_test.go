package v1

import (
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

func TestServer_FindGateways(t *testing.T) {
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
