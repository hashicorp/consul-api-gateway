package consul

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

func TestIntentionsReconciler_watchDiscoveryChain(t *testing.T) {
	require := require.New(t)
	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {

	})
	require.NoError(err)
	consulSrv.WaitForServiceIntentions(t)
	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	c, err := api.NewClient(cfg)
	require.NoError(err)
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan *discoChainWatchResult)
	t.Cleanup(func() {
		cancel()
		_ = consulSrv.Stop()
		close(results)
	})

	for _, name := range []string{"router", "upstream1", "upstream2"} {
		ok, _, err := c.ConfigEntries().Set(&api.ServiceConfigEntry{
			Kind:     api.ServiceDefaults,
			Name:     name,
			Protocol: "http",
		}, nil)
		require.True(ok)
		require.NoError(err)
	}

	ok, _, err := c.ConfigEntries().Set(&api.ServiceRouterConfigEntry{
		Kind: api.ServiceRouter,
		Name: "router",
		Routes: []api.ServiceRoute{
			{
				Match: &api.ServiceRouteMatch{
					HTTP: &api.ServiceRouteHTTPMatch{
						PathPrefix: "/1",
					},
				},
				Destination: &api.ServiceRouteDestination{
					Service: "upstream1",
				},
			},
			{
				Match: &api.ServiceRouteMatch{
					HTTP: &api.ServiceRouteHTTPMatch{
						PathPrefix: "/2",
					},
				},
				Destination: &api.ServiceRouteDestination{
					Service: "upstream2",
				},
			},
		},
	}, nil)
	require.True(ok)
	require.NoError(err)

	err = c.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Name:    "upstream1",
		Port:    9991,
		Address: "127.0.0.1",
	})
	require.NoError(err)
	err = c.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Name:    "upstream2",
		Port:    9992,
		Address: "127.0.0.1",
	})
	require.NoError(err)
	w := newDiscoChainWatcher(ctx, api.CompoundServiceName{Name: "router"}, results, c.DiscoveryChain(), testutil.Logger(t))

	var result *discoChainWatchResult
	require.Eventually(func() bool {
		var ok bool
		result, ok = <-results
		return ok
	}, 5*time.Second, 500*time.Millisecond)
	require.NotNil(results)
	require.Equal(w.name, result.name)
	require.Len(result.added, 2)
	require.Len(result.removed, 0)

	w.Cancel()
}
