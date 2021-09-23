package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/mocks"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	className = types.NamespacedName{
		Name:      "class",
		Namespace: "default",
	}
)

const mockControllerName = "mock.controller.name"

func TestGatewayClassSetup(t *testing.T) {
	require.Error(t, (&GatewayClassReconciler{}).SetupWithManager(nil))
}
func TestGatewayClass(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		err           error
		result        reconcile.Result
		expectationCB func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager)
	}{{
		name: "get-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(nil, errExpected)
		},
	}, {
		name: "deleted",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(nil, nil)
			reconciler.EXPECT().DeleteGatewayClass(className.Name)
		},
	}, {
		name: "not-managed",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController("other"),
				},
			}, nil)
		},
	}, {
		name: "deleting-in-use-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			now := meta.Now()
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(false, errExpected)
		},
	}, {
		name: "deleting-in-use",
		err:  errGatewayClassInUse,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			now := meta.Now()
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(true, nil)
		},
	}, {
		name: "deleting-finalizer-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			now := meta.Now()
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(false, nil)
			client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, errExpected)
		},
	}, {
		name: "deleting",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			now := meta.Now()
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().GatewayClassInUse(gomock.Any(), gomock.Any()).Return(false, nil)
			client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(true, nil)
		},
	}, {
		name: "create-finalizer-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, errExpected)
		},
	}, {
		name:   "create-finalizer-updated",
		result: reconcile.Result{Requeue: true},
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(true, nil)
		},
	}, {
		name: "create-validation-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
			client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(false, errExpected)
		},
	}, {
		name: "create-validation-true",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
			client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(true, nil)
			reconciler.EXPECT().UpsertGatewayClass(gomock.Any(), true)
		},
	}, {
		name: "create-validation-false",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
			client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(false, nil)
			reconciler.EXPECT().UpsertGatewayClass(gomock.Any(), false)
		},
	}, {
		name: "create-upsert-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetGatewayClass(gomock.Any(), className).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassFinalizer).Return(false, nil)
			client.EXPECT().IsValidGatewayClass(gomock.Any(), gomock.Any()).Return(true, nil)
			reconciler.EXPECT().UpsertGatewayClass(gomock.Any(), true).Return(errExpected)
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mocks.NewMockClient(ctrl)
			reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)
			if test.expectationCB != nil {
				test.expectationCB(client, reconciler)
			}

			controller := &GatewayClassReconciler{
				Client:         client,
				Log:            hclog.NewNullLogger(),
				ControllerName: mockControllerName,
				Manager:        reconciler,
			}
			result, err := controller.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: className,
			})
			if test.err != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.result, result)
		})
	}
}
