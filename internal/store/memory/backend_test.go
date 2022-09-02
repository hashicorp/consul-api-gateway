package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

func TestBackend_GetGateway(t *testing.T) {
	backend := NewBackend()

	gatewayID := core.GatewayID{
		ConsulNamespace: "default",
		Service:         t.Name(),
	}

	// Non-existent GatewayID should return error
	gateway, err := backend.GetGateway(context.Background(), gatewayID)
	assert.EqualError(t, err, store.ErrNotFound.Error())
	assert.Nil(t, gateway)

	// Existing GatewayID should return the corresponding Gateway
	backend.gateways[gatewayID] = []byte(t.Name())

	gateway, err = backend.GetGateway(context.Background(), gatewayID)
	assert.NoError(t, err)
	assert.Equal(t, gateway, []byte(t.Name()))
}

func TestBackend_ListGateways(t *testing.T) {
	backend := NewBackend()

	// Empty backend should return no Gateways
	gateways, err := backend.ListGateways(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, gateways)

	// Backend should return all upserted Gateways
	gateway1 := store.GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default1", Service: t.Name() + "_1"}, Data: []byte(t.Name() + "_1")}
	backend.gateways[gateway1.ID] = gateway1.Data

	gateways, err = backend.ListGateways(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, gateways, [][]byte{gateway1.Data})

	gateway2 := store.GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default2", Service: t.Name() + "_2"}, Data: []byte(t.Name() + "_2")}
	backend.gateways[gateway2.ID] = gateway2.Data

	gateways, err = backend.ListGateways(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, gateways, [][]byte{gateway1.Data, gateway2.Data})
}

func TestBackend_DeleteGateway(t *testing.T) {
	backend := NewBackend()

	// Empty backend should no-op
	assert.NoError(t, backend.DeleteGateway(context.Background(), core.GatewayID{ConsulNamespace: "default", Service: t.Name()}))

	// Delete should remove targeted Gateway from backend and leave others
	gateway1 := store.GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default1", Service: t.Name() + "_1"}, Data: []byte(t.Name() + "_1")}
	backend.gateways[gateway1.ID] = gateway1.Data

	gateway2 := store.GatewayRecord{ID: core.GatewayID{ConsulNamespace: "default2", Service: t.Name() + "_2"}, Data: []byte(t.Name() + "_2")}
	backend.gateways[gateway2.ID] = gateway2.Data

	err := backend.DeleteGateway(context.Background(), gateway1.ID)
	assert.NoError(t, err)
	assert.NotContains(t, backend.gateways, gateway1.ID)

	require.Contains(t, backend.gateways, gateway2.ID)
	assert.Equal(t, gateway2.Data, backend.gateways[gateway2.ID])
}
