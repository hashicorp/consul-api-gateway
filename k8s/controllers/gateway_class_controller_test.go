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
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const mockControllerName = "mock.controller.name"

func TestGatewayClass_GetError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(nil, expectedErr)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_Deleted(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(nil, nil)
	reconciler.EXPECT().DeleteGatewayClass(namespacedName.Name)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_NotManaged(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController("other"),
		},
	}, nil)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_DeletingInUseError(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(false, expectedErr)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_DeletingInUse(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(true, nil)

	controller := &GatewayClassReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	_, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "in use")
}

func TestGatewayClass_DeletingFinalizerError(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, expectedErr)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_Deleting(t *testing.T) {
	t.Parallel()

	now := meta.Now()
	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &now,
		},
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(true, nil)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_CreateFinalizerError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, expectedErr)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_CreateFinalizerUpdated(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(true, nil)

	controller := &GatewayClassReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconciler,
	}
	result, err := controller.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: namespacedName,
	})
	require.NoError(t, err)
	// ensure we requeue
	require.Equal(t, reconcile.Result{Requeue: true}, result)
}

func TestGatewayClass_CreateValidationError(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}
	expectedErr := errors.New("expected")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
	client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(false, expectedErr)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_CreateValidationTrue(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
	client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(true, nil)
	reconciler.EXPECT().UpsertGatewayClass(gomock.Any(), true)

	controller := &GatewayClassReconciler{
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

func TestGatewayClass_CreateValidationFalse(t *testing.T) {
	t.Parallel()

	namespacedName := types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockClient(ctrl)
	reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)

	client.EXPECT().GetGatewayClass(gomock.Any(), namespacedName).Return(&gateway.GatewayClass{
		Spec: gateway.GatewayClassSpec{
			Controller: gateway.GatewayController(mockControllerName),
		},
	}, nil)
	client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
	client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(false, nil)
	reconciler.EXPECT().UpsertGatewayClass(gomock.Any(), false)

	controller := &GatewayClassReconciler{
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
