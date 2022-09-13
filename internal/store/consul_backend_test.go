package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/require"
)

func testConsul(t *testing.T) *api.Client {
	t.Helper()

	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.Peering = nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = consulSrv.Stop()
	})
	consulSrv.WaitForLeader(t)

	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	consul, err := api.NewClient(cfg)
	require.NoError(t, err)
	return consul
}

func TestConsulStore_Gateways(t *testing.T) {
	t.Parallel()

	consul := testConsul(t)

	id := uuid.New().String()

	backend := NewConsulBackend(id, consul, "", "foo")
	require.NoError(t, backend.UpsertGateways(context.Background(), []GatewayRecord{{
		ID:   core.GatewayID{ConsulNamespace: "foo", Service: "foo"},
		Data: []byte("foo"),
	}}...))

	gateways, err := backend.ListGateways(context.Background())
	require.NoError(t, err)
	require.Len(t, gateways, 1)
	require.Contains(t, gateways, []byte("foo"))

	require.NoError(t, backend.UpsertGateways(context.Background(), []GatewayRecord{{
		ID:   core.GatewayID{ConsulNamespace: "bar", Service: "bar"},
		Data: []byte("bar"),
	}, {
		ID:   core.GatewayID{ConsulNamespace: "baz", Service: "baz"},
		Data: []byte("baz"),
	}}...))

	gateways, err = backend.ListGateways(context.Background())
	require.NoError(t, err)
	require.Len(t, gateways, 3)
	require.Contains(t, gateways, []byte("foo"))
	require.Contains(t, gateways, []byte("bar"))
	require.Contains(t, gateways, []byte("baz"))

	require.NoError(t, backend.DeleteGateway(context.Background(), core.GatewayID{ConsulNamespace: "bar", Service: "bar"}))

	gateways, err = backend.ListGateways(context.Background())
	require.NoError(t, err)
	require.Len(t, gateways, 2)
	require.Contains(t, gateways, []byte("foo"))
	require.Contains(t, gateways, []byte("baz"))

	gateway, err := backend.GetGateway(context.Background(), core.GatewayID{ConsulNamespace: "foo", Service: "foo"})
	require.NoError(t, err)
	require.Equal(t, gateway, []byte("foo"))
}

func TestConsulStore_Routes(t *testing.T) {
	t.Parallel()

	consul := testConsul(t)

	id := uuid.New().String()

	backend := NewConsulBackend(id, consul, "", "foo")
	require.NoError(t, backend.UpsertRoutes(context.Background(), []RouteRecord{{
		ID:   "foo",
		Data: []byte("foo"),
	}}...))

	routes, err := backend.ListRoutes(context.Background())
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.Contains(t, routes, []byte("foo"))

	require.NoError(t, backend.UpsertRoutes(context.Background(), []RouteRecord{{
		ID:   "bar",
		Data: []byte("bar"),
	}, {
		ID:   "baz",
		Data: []byte("baz"),
	}}...))

	routes, err = backend.ListRoutes(context.Background())
	require.NoError(t, err)
	require.Len(t, routes, 3)
	require.Contains(t, routes, []byte("foo"))
	require.Contains(t, routes, []byte("bar"))
	require.Contains(t, routes, []byte("baz"))

	require.NoError(t, backend.DeleteRoute(context.Background(), "bar"))

	routes, err = backend.ListRoutes(context.Background())
	require.NoError(t, err)
	require.Len(t, routes, 2)
	require.Contains(t, routes, []byte("foo"))
	require.Contains(t, routes, []byte("baz"))

	route, err := backend.GetRoute(context.Background(), "foo")
	require.NoError(t, err)
	require.Equal(t, route, []byte("foo"))
}
