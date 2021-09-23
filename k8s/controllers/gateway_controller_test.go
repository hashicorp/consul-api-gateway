package controllers

import (
	"context"
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

var (
	gatewayName = types.NamespacedName{
		Name:      "gateway",
		Namespace: "default",
	}
)

func TestGateway(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		err           error
		result        reconcile.Result
		expectationCB func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker)
	}{{
		name: "get-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(nil, errExpected)
		},
	}, {
		name: "deleted",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(nil, nil)
			reconciler.EXPECT().DeleteGateway(gatewayName)
			tracker.EXPECT().DeleteStatus(gatewayName)
		},
	}, {
		name: "gateway-class-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(nil, errExpected)
		},
	}, {
		name: "gateway-class-unmanaged",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController("other"),
				},
			}, nil)
		},
	}, {
		name: "deployment-exists-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, errExpected)
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mocks.NewMockClient(ctrl)
			reconciler := reconcilerMocks.NewMockReconcileManager(ctrl)
			tracker := reconcilerMocks.NewMockGatewayStatusTracker(ctrl)
			if test.expectationCB != nil {
				test.expectationCB(client, reconciler, tracker)
			}

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
				NamespacedName: gatewayName,
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
