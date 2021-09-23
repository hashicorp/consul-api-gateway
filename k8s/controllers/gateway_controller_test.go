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
)

func TestGateway_GetError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "gateway",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)
	tracker := reconcilerMocks.NewMockGatewayStatusTracker(ctrl)

	client.EXPECT().GetGateway(gomock.Any(), namespacedName).Return(nil, expectedErr)

	controller := &GatewayReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
		Tracker:        tracker,
		SDSServerHost:  "host",
		SDSServerPort:  1,
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestGateway_Deleted(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "route",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)
	tracker := reconcilerMocks.NewMockGatewayStatusTracker(ctrl)

	client.EXPECT().GetGateway(gomock.Any(), namespacedName).Return(nil, nil)
	reconciler.EXPECT().DeleteGateway(namespacedName)
	tracker.EXPECT().DeleteStatus(namespacedName)

	controller := &GatewayReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
		Tracker:        tracker,
		SDSServerHost:  "host",
		SDSServerPort:  1,
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}
