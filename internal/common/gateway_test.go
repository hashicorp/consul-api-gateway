package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewaySecretRegistry(t *testing.T) {
	registry := NewGatewaySecretRegistry()

	gatewayInfo := GatewayInfo{
		Namespace: "test",
		Service:   "test",
	}
	require.False(t, registry.GatewayExists(gatewayInfo))

	registry.AddGateway(gatewayInfo, "secretOne")
	require.True(t, registry.GatewayExists(gatewayInfo))
	require.True(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretOne"}))
	require.False(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretTwo"}))
	require.False(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretOne", "secretTwo"}))

	registry.AddSecrets(gatewayInfo, "secretTwo")
	require.True(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretTwo"}))
	require.True(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretOne", "secretTwo"}))

	registry.RemoveSecrets(gatewayInfo, "secretOne")
	require.False(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretOne"}))
	require.True(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretTwo"}))
	require.False(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretOne", "secretTwo"}))

	registry.RemoveGateway(gatewayInfo)
	require.False(t, registry.CanFetchSecrets(gatewayInfo, []string{"secretTwo"}))
}
