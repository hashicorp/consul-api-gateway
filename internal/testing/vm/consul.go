package vm

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/require"
)

type Consul struct {
	Client  *api.Client
	Config  *api.Config
	Node    string
	Token   string
	XDSPort int
}

func TestConsul(t *testing.T, acls bool) *Consul {
	t.Helper()

	token := uuid.New().String()

	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.ACL.Enabled = acls
		c.ACL.Tokens.InitialManagement = token
		c.Peering = nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = consulSrv.Stop()
	})
	consulSrv.WaitForLeader(t)

	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	cfg.Token = token
	consul, err := api.NewClient(cfg)
	require.NoError(t, err)

	return &Consul{
		Client:  consul,
		Token:   token,
		Node:    consulSrv.Config.NodeID,
		Config:  cfg,
		XDSPort: consulSrv.Config.Ports.GRPC,
	}
}
