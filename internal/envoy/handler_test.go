package envoy

import (
	"context"
	"errors"
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/envoy/mocks"
	"github.com/hashicorp/go-hclog"
)

func TestOnStreamRequest(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), gomock.Any(), requestedSecrets).Return(true, nil)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	request := &discovery.DiscoveryRequest{
		ResourceNames: requestedSecrets,
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().SetResourcesForNode(gomock.Any(), request.ResourceNames, request.Node.Id).Return(nil)

	err := handler.OnStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamRequest(1, request)
	require.NoError(t, err)
}

func TestOnStreamRequest_PermissionError(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), gomock.Any(), requestedSecrets).Return(false, nil)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	request := &discovery.DiscoveryRequest{
		ResourceNames: requestedSecrets,
		Node: &core.Node{
			Id: "1",
		},
	}

	err := handler.OnStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamRequest(1, request)
	require.Contains(t, err.Error(), "the current gateway does not have permission to fetch the requested secrets")
}

func TestOnStreamRequest_SetResourcesForNodeError(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedErr := errors.New("error")
	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), gomock.Any(), requestedSecrets).Return(true, nil)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	request := &discovery.DiscoveryRequest{
		ResourceNames: requestedSecrets,
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().SetResourcesForNode(gomock.Any(), request.ResourceNames, request.Node.Id).Return(expectedErr)

	err := handler.OnStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamRequest(1, request)
	require.Equal(t, expectedErr, err)
}

func TestOnStreamRequest_Graceful(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), gomock.Any(), requestedSecrets).Return(true, nil)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	request := &discovery.DiscoveryRequest{
		ResourceNames: requestedSecrets,
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().SetResourcesForNode(gomock.Any(), request.ResourceNames, request.Node.Id).Return(nil)

	// without setting up the stream context in the open call
	err := handler.OnStreamRequest(1, request)
	require.NoError(t, err)
}

func TestOnStreamClosed(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), gomock.Any(), requestedSecrets).Return(true, nil)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	request := &discovery.DiscoveryRequest{
		ResourceNames: requestedSecrets,
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().SetResourcesForNode(gomock.Any(), request.ResourceNames, request.Node.Id).Return(nil)
	secrets.EXPECT().UnwatchAll(gomock.Any(), request.Node.Id)

	err := handler.OnStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamRequest(1, request)
	require.NoError(t, err)
	handler.OnStreamClosed(1)
}

func TestOnStreamClosed_Graceful(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	// no-ops instead of panics without setting up the stream context in the open call
	handler.OnStreamClosed(1)
}

func TestOnStreamOpen(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewaySecretRegistry(ctrl)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, secrets)

	// errors on non secret requests
	err := handler.OnStreamOpen(context.Background(), 1, resource.ClusterType)
	require.Error(t, err)
}
