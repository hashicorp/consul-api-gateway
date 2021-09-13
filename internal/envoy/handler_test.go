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

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/envoy/mocks"
	"github.com/hashicorp/polar/internal/metrics"
)

func TestOnStreamDeltaRequest(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), requestedSecrets).Return(true)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	request := &discovery.DeltaDiscoveryRequest{
		ResourceNamesSubscribe: requestedSecrets,
		ResourceNamesUnsubscribe: []string{
			"c",
			"d",
		},
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().Watch(gomock.Any(), request.ResourceNamesSubscribe, request.Node.Id).Return(nil)
	secrets.EXPECT().Unwatch(gomock.Any(), request.ResourceNamesUnsubscribe, request.Node.Id).Return(nil)

	err := handler.OnDeltaStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamDeltaRequest(1, request)
	require.NoError(t, err)
}

func TestOnStreamDeltaRequest_PermissionError(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), requestedSecrets).Return(false)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	request := &discovery.DeltaDiscoveryRequest{
		ResourceNamesSubscribe: requestedSecrets,
		ResourceNamesUnsubscribe: []string{
			"c",
			"d",
		},
		Node: &core.Node{
			Id: "1",
		},
	}

	err := handler.OnDeltaStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamDeltaRequest(1, request)
	require.Contains(t, err.Error(), "the current gateway does not have permission to fetch the requested secrets")
}

func TestOnStreamDeltaRequest_WatchError(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedErr := errors.New("error")
	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), requestedSecrets).Return(true)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	request := &discovery.DeltaDiscoveryRequest{
		ResourceNamesSubscribe: requestedSecrets,
		ResourceNamesUnsubscribe: []string{
			"c",
			"d",
		},
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().Watch(gomock.Any(), request.ResourceNamesSubscribe, request.Node.Id).Return(expectedErr)

	err := handler.OnDeltaStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamDeltaRequest(1, request)
	require.Equal(t, expectedErr, err)
}

func TestOnStreamDeltaRequest_UnwatchError(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedErr := errors.New("error")
	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), requestedSecrets).Return(true)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	request := &discovery.DeltaDiscoveryRequest{
		ResourceNamesSubscribe: requestedSecrets,
		ResourceNamesUnsubscribe: []string{
			"c",
			"d",
		},
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().Watch(gomock.Any(), request.ResourceNamesSubscribe, request.Node.Id).Return(nil)
	secrets.EXPECT().Unwatch(gomock.Any(), request.ResourceNamesUnsubscribe, request.Node.Id).Return(expectedErr)

	err := handler.OnDeltaStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamDeltaRequest(1, request)
	require.Equal(t, expectedErr, err)
}

func TestOnStreamDeltaRequest_Graceful(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), requestedSecrets).Return(true)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	request := &discovery.DeltaDiscoveryRequest{
		ResourceNamesSubscribe: requestedSecrets,
		ResourceNamesUnsubscribe: []string{
			"c",
			"d",
		},
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().Watch(gomock.Any(), request.ResourceNamesSubscribe, request.Node.Id).Return(nil)
	secrets.EXPECT().Unwatch(gomock.Any(), request.ResourceNamesUnsubscribe, request.Node.Id).Return(nil)

	// without setting up the stream context in the open call
	err := handler.OnStreamDeltaRequest(1, request)
	require.NoError(t, err)
}

func TestOnDeltaStreamClosed(t *testing.T) {
	t.Parallel()

	requestedSecrets := []string{
		"a",
		"b",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	registry.EXPECT().CanFetchSecrets(gomock.Any(), requestedSecrets).Return(true)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	request := &discovery.DeltaDiscoveryRequest{
		ResourceNamesSubscribe: requestedSecrets,
		ResourceNamesUnsubscribe: []string{
			"c",
			"d",
		},
		Node: &core.Node{
			Id: "1",
		},
	}
	secrets.EXPECT().Watch(gomock.Any(), request.ResourceNamesSubscribe, request.Node.Id).Return(nil)
	secrets.EXPECT().Unwatch(gomock.Any(), request.ResourceNamesUnsubscribe, request.Node.Id).Return(nil)
	secrets.EXPECT().UnwatchAll(gomock.Any(), request.Node.Id)

	err := handler.OnDeltaStreamOpen(context.Background(), 1, resource.SecretType)
	require.NoError(t, err)
	err = handler.OnStreamDeltaRequest(1, request)
	require.NoError(t, err)
	handler.OnDeltaStreamClosed(1)
}

func TestOnDeltaStreamClosed_Graceful(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	// no-ops instead of panics without setting up the stream context in the open call
	handler.OnDeltaStreamClosed(1)
}

func TestOnDeltaStreamOpen(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secrets := mocks.NewMockSecretManager(ctrl)
	registry := mocks.NewMockGatewayRegistry(ctrl)
	handler := NewRequestHandler(hclog.NewNullLogger(), registry, metrics.Registry.SDS, secrets)

	// errors on non secret requests
	err := handler.OnDeltaStreamOpen(context.Background(), 1, resource.ClusterType)
	require.Error(t, err)
}
