package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestUpsertGatewayClass(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	manager := NewReconcileManager(ManagerConfig{
		Client: client,
		Logger: hclog.NewNullLogger(),
	})

	inner := &gw.GatewayClass{}
	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.Equal(t, expected, manager.UpsertGatewayClass(context.Background(), inner))

	client.EXPECT().UpdateStatus(gomock.Any(), inner)
	require.NoError(t, manager.UpsertGatewayClass(context.Background(), inner))

	// validation
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.Equal(t, expected, manager.UpsertGatewayClass(context.Background(), &gw.GatewayClass{
		Spec: gw.GatewayClassSpec{
			ParametersRef: &gw.ParametersReference{
				Group: apigwv1alpha1.Group,
				Kind:  apigwv1alpha1.GatewayClassConfigKind,
			},
		},
	}))

}

func TestUpsertGateway(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	store := storeMocks.NewMockStore(ctrl)

	manager := NewReconcileManager(ManagerConfig{
		Client: client,
		Logger: hclog.NewNullLogger(),
		Store:  store,
	})

	inner := &gw.Gateway{}
	expected := errors.New("expected")

	client.EXPECT().HasManagedDeployment(gomock.Any(), inner).Return(false, expected)
	require.Equal(t, expected, manager.UpsertGateway(context.Background(), inner))

	client.EXPECT().HasManagedDeployment(gomock.Any(), inner).Return(false, nil)
	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, false, expected)
	require.Equal(t, expected, manager.UpsertGateway(context.Background(), inner))

	client.EXPECT().HasManagedDeployment(gomock.Any(), inner).Return(false, nil)
	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, false, nil)
	require.NoError(t, manager.UpsertGateway(context.Background(), inner))

	// validation
	client.EXPECT().HasManagedDeployment(gomock.Any(), inner).Return(false, nil)
	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, true, nil)
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.Equal(t, expected, manager.UpsertGateway(context.Background(), inner))

	client.EXPECT().HasManagedDeployment(gomock.Any(), inner).Return(false, nil)
	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, true, nil)
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(&apps.Deployment{}, nil)
	store.EXPECT().UpsertGateway(gomock.Any(), gomock.Any())
	require.NoError(t, manager.UpsertGateway(context.Background(), inner))
}

func TestUpsertHTTPRoute(t *testing.T) {
	t.Parallel()

	// UpsertHTTPRoute(ctx context.Context, r Route) error
}

func TestUpsertTCPRoute(t *testing.T) {
	t.Parallel()

	// UpsertTCPRoute(ctx context.Context, r Route) error
}

func TestUpsertTLSRoute(t *testing.T) {
	t.Parallel()

	// UpsertTLSRoute(ctx context.Context, r Route) error
}

func TestDeleteGatewayClass(t *testing.T) {
	t.Parallel()

	require.NoError(t, NewReconcileManager(ManagerConfig{Logger: hclog.NewNullLogger()}).DeleteGatewayClass(context.Background(), ""))
}

func TestDeleteGateway(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := storeMocks.NewMockStore(ctrl)

	manager := NewReconcileManager(ManagerConfig{
		Logger: hclog.NewNullLogger(),
		Store:  store,
	})

	expected := errors.New("expected")

	store.EXPECT().DeleteGateway(gomock.Any(), gomock.Any()).Return(expected)
	require.Equal(t, expected, manager.DeleteGateway(context.Background(), types.NamespacedName{}))

	store.EXPECT().DeleteGateway(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.DeleteGateway(context.Background(), types.NamespacedName{}))
}

func TestDeleteHTTPRoute(t *testing.T) {
	t.Parallel()

	// DeleteHTTPRoute(ctx context.Context, name types.NamespacedName) error
}

func TestDeleteTCPRoute(t *testing.T) {
	t.Parallel()

	// DeleteTCPRoute(ctx context.Context, name types.NamespacedName) error
}

func TestDeleteTLSRoute(t *testing.T) {
	t.Parallel()

	// DeleteTLSRoute(ctx context.Context, name types.NamespacedName) error
}
