package controllers

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/k8s/reconciler/mocks"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestHTTPRoute_GetError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "route",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetHTTPRoute(gomock.Any(), namespacedName).Return(nil, expectedErr)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestHTTPRoute_Deleted(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "route",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetHTTPRoute(gomock.Any(), namespacedName).Return(nil, nil)
	reconciler.EXPECT().DeleteRoute(namespacedName)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}

func TestHTTPRoute_ManagedError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "route",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetHTTPRoute(gomock.Any(), namespacedName).Return(&gateway.HTTPRoute{}, nil)
	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), mockControllerName).Return(false, expectedErr)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestHTTPRoute_NotManaged(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "route",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetHTTPRoute(gomock.Any(), namespacedName).Return(&gateway.HTTPRoute{}, nil)
	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), mockControllerName).Return(false, nil)
	reconciler.EXPECT().DeleteRoute(namespacedName)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}

func TestHTTPRoute_Managed(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "route",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetHTTPRoute(gomock.Any(), namespacedName).Return(&gateway.HTTPRoute{}, nil)
	client.EXPECT().IsManagedRoute(gomock.Any(), gomock.Any(), mockControllerName).Return(true, nil)
	reconciler.EXPECT().UpsertHTTPRoute(gomock.Any())

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}
