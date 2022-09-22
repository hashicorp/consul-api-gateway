package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
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

	gw := &gwv1beta1.Gateway{}

	gateway := newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	gateway.Gateway.Status = gateway.GatewayState.GetStatus(gw)
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	assert.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	assert.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	assert.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	expected := errors.New("expected")

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	assert.True(t, errors.Is(updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}), expected))

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	assert.True(t, errors.Is(updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}), expected))

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(expected)
	assert.Equal(t, expected, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return false, nil
	}))

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	assert.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
		return true, nil
	}))

	gw = &gwv1beta1.Gateway{}
	gateway = newK8sGateway(apigwv1alpha1.GatewayClassConfig{}, gw, state.InitialGatewayState(gw))
	client.EXPECT().CreateOrUpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	assert.NoError(t, updater.UpdateGatewayStatusOnSync(context.Background(), gateway, func() (bool, error) {
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

	inner := &gwv1alpha2.TCPRoute{
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gwv1alpha2.TCPRouteStatus{
			RouteStatus: gwv1alpha2.RouteStatus{
				Parents: []gwv1alpha2.RouteParentStatus{{
					ParentRef: gwv1alpha2.ParentReference{
						Name: "expected",
					},
					ControllerName: "expected",
				}, {
					ParentRef: gwv1alpha2.ParentReference{
						Name: "expected",
					},
					ControllerName: "other",
				}, {
					ParentRef: gwv1alpha2.ParentReference{
						Name: "other",
					},
					ControllerName: "other",
				}},
			},
		},
	}

	route := newK8sRoute(inner, state.NewRouteState())
	route.RouteState.Bound(gwv1alpha2.ParentReference{
		Name: "expected",
	})

	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.True(t, errors.Is(updater.UpdateRouteStatus(context.Background(), route), expected))

	require.NoError(t, updater.UpdateRouteStatus(context.Background(), route))
}
