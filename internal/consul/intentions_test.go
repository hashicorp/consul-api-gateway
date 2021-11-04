package consul

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cenkalti/backoff"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/consul/mocks"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

type configEntryMatcher struct {
	Kind      string
	Name      string
	Namespace string
}

func matchConfigEntry(kind, namespace, name string) gomock.Matcher {
	return &configEntryMatcher{Kind: kind, Name: name, Namespace: namespace}
}

func (m *configEntryMatcher) Matches(arg interface{}) bool {
	entry, ok := arg.(api.ConfigEntry)
	if !ok {
		fmt.Println("BAD type")
		return false
	}

	return m.Kind == entry.GetKind() && m.Name == entry.GetName() && m.Namespace == entry.GetNamespace()
}

func (m *configEntryMatcher) String() string {
	return fmt.Sprintf("{Kind: %q, Name: %q, Namespace: %q}", m.Kind, m.Name, m.Namespace)
}

func testIntentionsReconciler(t *testing.T, disco consulDiscoveryChains, config consulConfigEntries) *IntentionsReconciler {
	return newIntentionsReconciler(disco, config, api.CompoundServiceName{Name: "name1", Namespace: "namespace1"}, testutil.Logger(t))
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
		require.NoError(st, r.syncIntentions())
	})
	t.Run("single target", func(st *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := mocks.NewMockconsulConfigEntries(ctrl)
		r := testIntentionsReconciler(t, nil, config)

		r.targetIndex.addRef(api.CompoundServiceName{Name: "foo"}, api.CompoundServiceName{Name: "source1"})
		config.EXPECT().Get(gomock.Eq(api.ServiceIntentions), gomock.Eq("foo"), gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind: api.ServiceIntentions,
			Name: "foo",
		}, &api.QueryMeta{LastIndex: 1}, nil).Times(1)
		config.EXPECT().CAS(gomock.Any(), gomock.Any(), gomock.Nil()).Return(true, nil, nil).Times(1)
		require.NoError(st, r.syncIntentions())
		require.Len(st, r.targetIndex.all(), 1)
	})
	t.Run("single target, already exists", func(st *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := mocks.NewMockconsulConfigEntries(ctrl)
		r := testIntentionsReconciler(t, nil, config)

		r.targetIndex.addRef(api.CompoundServiceName{Name: "foo"}, api.CompoundServiceName{Name: "source1"})
		config.EXPECT().Get(gomock.Eq(api.ServiceIntentions), gomock.Eq("foo"), gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind:    api.ServiceIntentions,
			Name:    "foo",
			Sources: []*api.SourceIntention{r.sourceIntention()},
		}, &api.QueryMeta{LastIndex: 1}, nil).Times(1)
		config.EXPECT().CAS(gomock.Any(), gomock.Any(), gomock.Nil()).Return(true, nil, nil).Times(0)
		require.NoError(st, r.syncIntentions())
		require.Len(st, r.targetIndex.all(), 1)
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
		require.NoError(st, r.syncIntentions())
		require.Len(st, r.targetTombstones.All(), 0)
	})
}

func TestIntentionsReconciler_handleChainResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// skip backoff for testing
	updateIntentionsRetryInterval = backoff.Stop

	mkChain := func(name string, added, removed []string) *discoChainWatchResult {
		result := &discoChainWatchResult{
			name:    api.CompoundServiceName{Name: name},
			added:   make([]api.CompoundServiceName, len(added)),
			removed: make([]api.CompoundServiceName, len(removed)),
		}
		for i, n := range added {
			result.added[i] = api.CompoundServiceName{Name: n}
		}
		for i, n := range removed {
			result.removed[i] = api.CompoundServiceName{Name: n}
		}
		return result
	}

	config := mocks.NewMockconsulConfigEntries(ctrl)
	r := testIntentionsReconciler(t, nil, config)
	require.Len(t, r.targetIndex.all(), 0)
	r.handleChainResult(mkChain("router", []string{}, []string{}))
	require.Len(t, r.targetIndex.all(), 0)

	gomock.InOrder(
		config.EXPECT().Get(api.ServiceIntentions, "t1", gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind: api.ServiceIntentions,
			Name: "t1",
			Sources: []*api.SourceIntention{
				{
					Name:   "foo",
					Action: api.IntentionActionAllow,
				},
			},
		}, &api.QueryMeta{LastIndex: 1}, nil),
		config.EXPECT().CAS(matchConfigEntry(api.ServiceIntentions, "", "t1"), gomock.Any(), gomock.Any()).Return(true, nil, nil),
	)
	r.handleChainResult(mkChain("router", []string{"t1"}, []string{}))
	require.NoError(t, r.syncIntentions())
	require.Len(t, r.targetIndex.all(), 1)
	require.Len(t, r.targetTombstones.All(), 0)

	config = mocks.NewMockconsulConfigEntries(ctrl)
	r.consulConfig = config
	gomock.InOrder(
		config.EXPECT().Get(api.ServiceIntentions, "t2", gomock.Any()).Return(nil, nil, errors.New("Unexpected response code: 404")),
		config.EXPECT().CAS(matchConfigEntry(api.ServiceIntentions, "", "t2"), gomock.Any(), gomock.Any()).Return(true, nil, nil),
		config.EXPECT().Get(api.ServiceIntentions, "t1", gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind: api.ServiceIntentions,
			Name: "t1",
			Sources: []*api.SourceIntention{
				{
					Name:      "name1",
					Namespace: "namespace1",
					Action:    api.IntentionActionAllow,
				},
			},
		}, &api.QueryMeta{LastIndex: 1}, nil),
		config.EXPECT().Delete(api.ServiceIntentions, "t1", gomock.Any()).Return(nil, errors.New("mock")),
	)
	r.handleChainResult(mkChain("router", []string{"t2"}, []string{"t1"}))
	require.Error(t, r.syncIntentions())
	require.Len(t, r.targetIndex.all(), 1)
	require.Len(t, r.targetTombstones.All(), 1)

	config = mocks.NewMockconsulConfigEntries(ctrl)
	r.consulConfig = config
	gomock.InOrder(
		config.EXPECT().Get(api.ServiceIntentions, "t1", gomock.Any()).Return(nil, nil, errors.New("Unexpected response code: 404")),
		config.EXPECT().CAS(matchConfigEntry(api.ServiceIntentions, "", "t1"), gomock.Any(), gomock.Any()).Return(false, nil, nil),
		config.EXPECT().Get(api.ServiceIntentions, "t2", gomock.Any()).Return(&api.ServiceIntentionsConfigEntry{
			Kind: api.ServiceIntentions,
			Name: "t1",
			Sources: []*api.SourceIntention{
				{
					Name:      "name1",
					Namespace: "namespace1",
					Action:    api.IntentionActionAllow,
				},
			},
		}, &api.QueryMeta{LastIndex: 1}, nil),
		config.EXPECT().Delete(api.ServiceIntentions, "t1", gomock.Any()).Return(nil, nil),
	)
	r.handleChainResult(mkChain("router", []string{"t1"}, []string{"t2"}))
	require.Error(t, r.syncIntentions())
	require.Len(t, r.targetIndex.all(), 1)
	require.Len(t, r.targetTombstones.All(), 0)
}

func TestIntentionsReconciler_Reconcile(t *testing.T) {
	require := require.New(t)
	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.Connect = map[string]interface{}{"enabled": true}
	})
	require.NoError(err)
	t.Cleanup(func() {
		_ = consulSrv.Stop()
	})
	consulSrv.WaitForServiceIntentions(t)
	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	c, err := api.NewClient(cfg)
	require.NoError(err)
	igw := &api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
		Name: "name1",
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
	}
	r := NewIntentionsReconciler(c, igw, testutil.Logger(t))
	require.NoError(r.Reconcile())

	err = c.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Name:    "upstream1",
		Port:    9999,
		Address: "127.0.0.1",
		Connect: &api.AgentServiceConnect{
			SidecarService: &api.AgentServiceRegistration{},
		},
	})
	require.NoError(err)

	ok, _, err := c.ConfigEntries().Set(igw, nil)
	require.True(ok)
	require.NoError(err)
	err = c.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Kind:    api.ServiceKindIngressGateway,
		Name:    r.gatewayName.Name,
		Port:    9998,
		Address: "127.0.0.1",
	})
	require.NoError(err)
	require.NoError(r.Reconcile())

	entry, _, err := c.ConfigEntries().Get(api.ServiceIntentions, "upstream1", nil)
	require.NoError(err)
	intention, ok := entry.(*api.ServiceIntentionsConfigEntry)
	require.True(ok)
	require.NotNil(intention)
	require.Len(intention.Sources, 1)
	require.Equal("name1", intention.Sources[0].Name)
	require.Equal(api.IntentionActionAllow, intention.Sources[0].Action)
	r.Stop()
}

func Test_sourceIntentionMatches(t *testing.T) {
	mkSrcInt := func(name, namespace string) *api.SourceIntention {
		return &api.SourceIntention{Name: name, Namespace: namespace}
	}
	for _, c := range []struct {
		a     *api.SourceIntention
		b     *api.SourceIntention
		match bool
	}{
		{
			a: mkSrcInt("foo", ""),
			b: mkSrcInt("bar", ""),
		},
		{
			a:     mkSrcInt("foo", ""),
			b:     mkSrcInt("foo", ""),
			match: true,
		},
		{
			a:     mkSrcInt("foo", "default"),
			b:     mkSrcInt("foo", ""),
			match: true,
		},
		{
			a: mkSrcInt("foo", "default"),
			b: mkSrcInt("foo", "bar"),
		},
	} {
		require.Equal(t, c.match, sourceIntentionMatches(c.a, c.b))
	}
}
