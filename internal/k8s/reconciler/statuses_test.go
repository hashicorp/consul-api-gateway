package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestStatuses_GatewayTrackSync(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	updater := NewStatusUpdater(hclog.NewNullLogger(), client, NewDeployer(DeployerConfig{
		Client: client,
		Logger: hclog.NewNullLogger(),
	}), "")

	gateway := NewGateway(v1alpha1.GatewayClassConfig{}, &gw.Gateway{}, state.InitialGatewayState("", &gw.Gateway{}))
	gateway.Gateway.Status = gateway.GatewayState.GetStatus(gateway.Gateway)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	require.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	gateway = NewGateway(v1alpha1.GatewayClassConfig{}, &gw.Gateway{}, state.InitialGatewayState("", &gw.Gateway{}))
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	expected := errors.New("expected")

	gateway = NewGateway(v1alpha1.GatewayClassConfig{}, &gw.Gateway{}, state.InitialGatewayState("", &gw.Gateway{}))
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	require.True(t, errors.Is(updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}), expected))

	gateway = NewGateway(v1alpha1.GatewayClassConfig{}, &gw.Gateway{}, state.InitialGatewayState("", &gw.Gateway{}))
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(expected)
	require.Equal(t, expected, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	gateway = NewGateway(v1alpha1.GatewayClassConfig{}, &gw.Gateway{}, state.InitialGatewayState("", &gw.Gateway{}))
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return true, nil
	}))

	gateway = NewGateway(v1alpha1.GatewayClassConfig{}, &gw.Gateway{}, state.InitialGatewayState("", &gw.Gateway{}))
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, expected
	}))
}

func TestStatuses_RouteSyncStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	updater := NewStatusUpdater(hclog.NewNullLogger(), client, NewDeployer(DeployerConfig{
		Client: client,
		Logger: hclog.NewNullLogger(),
	}), "")

	inner := &gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gw.TCPRouteStatus{
			RouteStatus: gw.RouteStatus{
				Parents: []gw.RouteParentStatus{{
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					ControllerName: "expected",
				}, {
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					ControllerName: "other",
				}, {
					ParentRef: gw.ParentRef{
						Name: "other",
					},
					ControllerName: "other",
				}},
			},
		},
	}
	route := NewRoute(inner, state.NewRouteState())
	route.RouteState.Bound(gw.ParentRef{
		Name: "expected",
	})

	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.True(t, errors.Is(updater.UpdateRouteStatus(context.Background(), route), expected))

	require.NoError(t, updater.UpdateRouteStatus(context.Background(), route))
}
