package consul

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/consul/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

var testCompiledDC = &api.CompiledDiscoveryChain{}

func testIntentionsReconciler(t *testing.T, disco consulDiscoveryChains, config consulConfigEntries) *IntentionsReconciler {
	return newIntentionsReconciler(disco, config, core.GatewayID{
		ConsulNamespace: "namespace1",
		Service:         "name1",
	}, testutil.Logger(t))
}

func mockCompiledDiscoChain(name, namespace string) *api.CompiledDiscoveryChain {
	return &api.CompiledDiscoveryChain{
		ServiceName: name,
		Namespace:   namespace,
		Targets:     map[string]*api.DiscoveryTarget{},
	}
}

func TestIntentionsReconciler_sourceIntention(t *testing.T) {
	r := testIntentionsReconciler(t, nil, nil)
	i := r.sourceIntention()
	require.NotNil(t, i)
	require.Equal(t, "name1", i.Name)
	require.Equal(t, "namespace1", i.Namespace)
	require.Equal(t, api.IntentionActionAllow, i.Action)
	require.Contains(t, i.Description, "Allow traffic from Consul API Gateway")
}

func TestIntentionsReconciler_syncIntentions(t *testing.T) {
	t.Run("empty_state", func(st *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := mocks.NewMockconsulConfigEntries(ctrl)
		r := testIntentionsReconciler(t, nil, config)

		config.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		r.syncIntentions()
	})
	t.Run("single target", func(st *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := mocks.NewMockconsulConfigEntries(ctrl)
		r := testIntentionsReconciler(t, nil, config)

		r.targetIndex.Add(api.CompoundServiceName{Name: "foo"})
		config.EXPECT().Get(gomock.Eq(api.ServiceIntentions), gomock.Eq("foo"), gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind: api.ServiceIntentions,
			Name: "foo",
		}, &api.QueryMeta{LastIndex: 1}, nil).Times(1)
		config.EXPECT().CAS(gomock.Any(), gomock.Any(), gomock.Nil()).Return(true, nil, nil).Times(1)
		r.syncIntentions()
		require.Len(t, r.targetIndex.All(), 1)
	})
	t.Run("single target, already exists", func(st *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := mocks.NewMockconsulConfigEntries(ctrl)
		r := testIntentionsReconciler(t, nil, config)

		r.targetIndex.Add(api.CompoundServiceName{Name: "foo"})
		config.EXPECT().Get(gomock.Eq(api.ServiceIntentions), gomock.Eq("foo"), gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind:    api.ServiceIntentions,
			Name:    "foo",
			Sources: []*api.SourceIntention{r.sourceIntention()},
		}, &api.QueryMeta{LastIndex: 1}, nil).Times(1)
		config.EXPECT().CAS(gomock.Any(), gomock.Any(), gomock.Nil()).Return(true, nil, nil).Times(0)
		r.syncIntentions()
		require.Len(t, r.targetIndex.All(), 1)
	})
	t.Run("single tombstone", func(st *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := mocks.NewMockconsulConfigEntries(ctrl)
		r := testIntentionsReconciler(t, nil, config)

		r.targetTombstones.Add(api.CompoundServiceName{Name: "foo"})
		config.EXPECT().Get(gomock.Eq(api.ServiceIntentions), gomock.Eq("foo"), gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind:    api.ServiceIntentions,
			Name:    "foo",
			Sources: []*api.SourceIntention{r.sourceIntention()},
		}, &api.QueryMeta{LastIndex: 1}, nil).Times(1)
		config.EXPECT().Delete(api.ServiceIntentions, "foo", gomock.Any()).Return(nil, nil).Times(1)
		r.syncIntentions()
		require.Len(t, r.targetTombstones.All(), 0)
	})
}

func TestIntentionsReconciler_watchDiscoveryChain(t *testing.T) {
	require := require.New(t)
	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {

	})
	require.NoError(err)
	defer consulSrv.Stop()
	consulSrv.WaitForLeader(t)
	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	c, err := api.NewClient(cfg)
	require.NoError(err)
	r := testIntentionsReconciler(t, c.DiscoveryChain(), c.ConfigEntries())
	r.serviceName.ConsulNamespace = ""

	err = c.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Name:    "upstream1",
		Port:    9999,
		Address: "127.0.0.1",
	})
	require.NoError(err)

	ok, _, err := c.ConfigEntries().Set(&api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
		Name: r.serviceName.Service,
		TLS:  api.GatewayTLSConfig{},
		Listeners: []api.IngressListener{
			{
				Port:     7777,
				Protocol: "tcp",
				Services: []api.IngressService{
					{
						Name: "upstream1",
					},
				},
			},
		},
	}, nil)
	require.True(ok)
	require.NoError(err)

	ch := r.watchDiscoveryChain()
	var chain *api.CompiledDiscoveryChain
	require.Eventually(func() bool {
		var ok bool
		chain, ok = <-ch
		return ok
	}, 5*time.Second, 500*time.Millisecond)

	require.Equal(r.serviceName.Service, chain.ServiceName)
	require.Len(chain.Targets, 1)
	r.Stop()
	val, ok := <-ch
	require.False(ok)
	require.Nil(val)
}
