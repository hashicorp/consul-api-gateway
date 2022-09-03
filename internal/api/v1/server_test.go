package v1

import (
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
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
