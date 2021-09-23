package controllers

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestGatewayClassConfig_GetError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(nil, expectedErr)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestGatewayClassConfig_Deleted(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(nil, nil)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}

func TestGatewayClassConfig_DeletingInUseError(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(&apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
	}, nil)
	client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(false, expectedErr)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestGatewayClassConfig_DeletingInUse(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(&apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
	}, nil)
	client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(true, nil)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "in use")
}

func TestGatewayClassConfig_DeletingFinalizerError(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(&apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
	}, nil)
	client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(false, expectedErr)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestGatewayClassConfig_Deleting(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(&apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
	}, nil)
	client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(true, nil)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}

func TestGatewayClassConfig_CreateFinalizerError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(false, expectedErr)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestGatewayClassConfig_Create(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), namespacedName).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(true, nil)

	controller := &GatewayClassConfigReconciler{
		Client: client,
		Log:    hclog.NewNullLogger(),
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}
