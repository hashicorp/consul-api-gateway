// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

func TestBackend_UpsertGateways(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	gatewayID := core.GatewayID{
		ConsulNamespace: "default",
		Service:         t.Name(),
	}

	assert.NoError(t, backend.UpsertGateways(context.Background(), GatewayRecord{
		ID:   gatewayID,
		Data: []byte(t.Name()),
	}))

	require.Contains(t, backend.gateways, gatewayID)
	assert.Equal(t, []byte(t.Name()), backend.gateways[gatewayID])
}

func TestBackend_GetGateway(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	gatewayID := core.GatewayID{
		ConsulNamespace: "default",
		Service:         t.Name(),
	}

	// Empty backend should return error
	gateway, err := backend.GetGateway(context.Background(), gatewayID)
	assert.EqualError(t, err, ErrNotFound.Error())
	assert.Nil(t, gateway)

	// Existing GatewayID should return the corresponding Gateway
	backend.gateways[gatewayID] = []byte(t.Name())

	gateway, err = backend.GetGateway(context.Background(), gatewayID)
	assert.NoError(t, err)
	assert.Equal(t, gateway, []byte(t.Name()))
}

func TestBackend_ListGateways(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	// Empty backend should return no Gateways
	gateways, err := backend.ListGateways(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, gateways)

	// memoryBackend should return all upserted Gateways
	gateway1 := GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default1", Service: t.Name() + "_1"}, Data: []byte(t.Name() + "_1")}
	backend.gateways[gateway1.ID] = gateway1.Data

	gateways, err = backend.ListGateways(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{gateway1.Data}, gateways)

	gateway2 := GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default2", Service: t.Name() + "_2"}, Data: []byte(t.Name() + "_2")}
	backend.gateways[gateway2.ID] = gateway2.Data

	gateways, err = backend.ListGateways(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{gateway1.Data, gateway2.Data}, gateways)
}

func TestBackend_DeleteGateway(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	// Empty backend should no-op
	assert.NoError(t, backend.DeleteGateway(context.Background(), core.GatewayID{ConsulNamespace: "default", Service: t.Name()}))

	// Delete should remove targeted Gateway from backend and leave others
	gateway1 := GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default1", Service: t.Name() + "_1"}, Data: []byte(t.Name() + "_1")}
	backend.gateways[gateway1.ID] = gateway1.Data

	gateway2 := GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default2", Service: t.Name() + "_2"}, Data: []byte(t.Name() + "_2")}
	backend.gateways[gateway2.ID] = gateway2.Data

	err := backend.DeleteGateway(context.Background(), gateway1.ID)
	assert.NoError(t, err)
	assert.NotContains(t, backend.gateways, gateway1.ID)

	require.Contains(t, backend.gateways, gateway2.ID)
	assert.Equal(t, gateway2.Data, backend.gateways[gateway2.ID])
}

func TestBackend_UpsertRoutes(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	routeID := t.Name()

	assert.NoError(t, backend.UpsertRoutes(context.Background(), RouteRecord{
		ID:   routeID,
		Data: []byte(t.Name()),
	}))

	require.Contains(t, backend.routes, routeID)
	assert.Equal(t, []byte(t.Name()), backend.routes[routeID])
}

func TestBackend_GetRoute(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	routeID := t.Name()

	//Empty backend should return error
	route, err := backend.GetRoute(context.Background(), routeID)
	assert.EqualError(t, err, ErrNotFound.Error())
	assert.Nil(t, route)

	// Existing Route ID should return the corresponding Route
	backend.routes[routeID] = []byte(t.Name())

	route, err = backend.GetRoute(context.Background(), routeID)
	assert.NoError(t, err)
	assert.Equal(t, route, []byte(t.Name()))
}

func TestBackend_ListRoutes(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	// Empty backend should return no Routes
	routes, err := backend.ListRoutes(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, routes)

	// memoryBackend should return all inserted Routes
	route1 := RouteRecord{ID: t.Name() + "_1", Data: []byte(t.Name())}
	backend.routes[route1.ID] = route1.Data

	routes, err = backend.ListRoutes(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{route1.Data}, routes)

	route2 := RouteRecord{ID: t.Name() + "_2", Data: []byte(t.Name())}
	backend.routes[route2.ID] = route2.Data

	routes, err = backend.ListRoutes(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{route1.Data, route2.Data}, routes)
}

func TestBackend_DeleteRoute(t *testing.T) {
	t.Parallel()

	backend := NewMemoryBackend()

	// Empty backend should no-op
	assert.NoError(t, backend.DeleteRoute(context.Background(), t.Name()))

	// Delete should remove targeted Route from backend and leave others
	route1 := RouteRecord{ID: t.Name() + "_1", Data: []byte(t.Name() + "_1")}
	backend.routes[route1.ID] = route1.Data

	route2 := RouteRecord{ID: t.Name() + "_2", Data: []byte(t.Name() + "_2")}
	backend.routes[route2.ID] = route2.Data

	err := backend.DeleteRoute(context.Background(), route1.ID)
	assert.NoError(t, err)
	assert.NotContains(t, backend.routes, route1.ID)

	require.Contains(t, backend.routes, route2.ID)
	assert.Equal(t, route2.Data, backend.routes[route2.ID])
}
