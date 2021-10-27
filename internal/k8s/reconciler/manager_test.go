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

	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, false, expected)
	require.Equal(t, expected, manager.UpsertGateway(context.Background(), inner))

	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, false, nil)
	require.NoError(t, manager.UpsertGateway(context.Background(), inner))

	// annotation
	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, true, nil)
	client.EXPECT().Update(gomock.Any(), gomock.Any()).Return(expected)
	require.Equal(t, expected, manager.UpsertGateway(context.Background(), inner.DeepCopy()))

	client.EXPECT().GetConfigForGatewayClassName(gomock.Any(), "").Return(apigwv1alpha1.GatewayClassConfig{}, true, nil)
	client.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.UpsertGateway(context.Background(), inner))
	require.NotEmpty(t, inner.Annotations[annotationConfig])

	// validation
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.Equal(t, expected, manager.UpsertGateway(context.Background(), inner))

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil)
	store.EXPECT().UpsertGateway(gomock.Any(), gomock.Any())
	require.NoError(t, manager.UpsertGateway(context.Background(), inner))
}

func TestUpsertHTTPRoute(t *testing.T) {
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

	expected := errors.New("expected")

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	require.Equal(t, expected, manager.UpsertHTTPRoute(context.Background(), &gw.HTTPRoute{}))

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(expected)
	require.Equal(t, expected, manager.UpsertHTTPRoute(context.Background(), &gw.HTTPRoute{}))

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.UpsertHTTPRoute(context.Background(), &gw.HTTPRoute{}))

	manager.namespaceMap[types.NamespacedName{Name: "gateway"}] = ""
	store.EXPECT().UpsertRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.UpsertHTTPRoute(context.Background(), &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "gateway",
				}},
			},
		},
	}))

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	store.EXPECT().UpsertRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.UpsertHTTPRoute(context.Background(), &gw.HTTPRoute{}))

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	client.EXPECT().GetService(gomock.Any(), gomock.Any()).Return(nil, expected)
	port := gw.PortNumber(1)
	require.Equal(t, expected, manager.UpsertHTTPRoute(context.Background(), &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: gw.BackendObjectReference{
							Name: "name",
							Port: &port,
						},
					},
				}},
			}},
		},
	}))
}

func TestUpsertTCPRoute(t *testing.T) {
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

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	store.EXPECT().UpsertRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.UpsertTCPRoute(context.Background(), &gw.TCPRoute{}))
}

func TestUpsertTLSRoute(t *testing.T) {
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

	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	store.EXPECT().UpsertRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.UpsertTLSRoute(context.Background(), &gw.TLSRoute{}))
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

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := storeMocks.NewMockStore(ctrl)

	manager := NewReconcileManager(ManagerConfig{
		Logger: hclog.NewNullLogger(),
		Store:  store,
	})

	expected := errors.New("expected")

	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(expected)
	require.Equal(t, expected, manager.DeleteHTTPRoute(context.Background(), types.NamespacedName{}))

	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.DeleteHTTPRoute(context.Background(), types.NamespacedName{}))
}

func TestDeleteTCPRoute(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := storeMocks.NewMockStore(ctrl)

	manager := NewReconcileManager(ManagerConfig{
		Logger: hclog.NewNullLogger(),
		Store:  store,
	})

	expected := errors.New("expected")

	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(expected)
	require.Equal(t, expected, manager.DeleteTCPRoute(context.Background(), types.NamespacedName{}))

	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.DeleteTCPRoute(context.Background(), types.NamespacedName{}))
}

func TestDeleteTLSRoute(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := storeMocks.NewMockStore(ctrl)

	manager := NewReconcileManager(ManagerConfig{
		Logger: hclog.NewNullLogger(),
		Store:  store,
	})

	expected := errors.New("expected")

	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(expected)
	require.Equal(t, expected, manager.DeleteTLSRoute(context.Background(), types.NamespacedName{}))

	store.EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, manager.DeleteTLSRoute(context.Background(), types.NamespacedName{}))
}
