package vm

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/require"
)

type Consul struct {
	Client      *api.Client
	Config      *api.Config
	ConnectCert string
	Node        string
	Token       string
	XDSPort     int
}

func TestConsul(t *testing.T, acls bool) *Consul {
	t.Helper()

	token := uuid.New().String()

	file, err := os.CreateTemp("", "bootstrap")
	require.NoError(t, err)
	certPath := file.Name()
	require.NoError(t, file.Close())

	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.ACL.Enabled = acls
		c.ACL.Tokens.InitialManagement = token
		c.Peering = nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = consulSrv.Stop()
		_ = os.Remove(certPath)
	})
	consulSrv.WaitForLeader(t)
	consulSrv.WaitForActiveCARoot(t)

	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	cfg.Token = token
	consul, err := api.NewClient(cfg)
	require.NoError(t, err)

	roots, _, err := consul.Connect().CARoots(nil)
	require.NoError(t, err)
	for _, root := range roots.Roots {
		if root.Active {
			require.NoError(t, os.WriteFile(certPath, []byte(root.RootCertPEM), 0644))
		}
	}

	return &Consul{
		Client:      consul,
		Token:       token,
		Node:        consulSrv.Config.NodeID,
		Config:      cfg,
		XDSPort:     consulSrv.Config.Ports.GRPC,
		ConnectCert: certPath,
	}
}
