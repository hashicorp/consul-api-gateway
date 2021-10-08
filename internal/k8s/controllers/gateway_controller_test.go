package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/mocks"
	"github.com/hashicorp/go-hclog"
)

var (
	gatewayName = types.NamespacedName{
		Name:      "gateway",
		Namespace: "default",
	}
)

func TestGatewaySetup(t *testing.T) {
	require.Error(t, (&GatewayReconciler{}).SetupWithManager(nil))
}

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
			reconciler.EXPECT().DeleteGateway(gomock.Any(), gatewayName)
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
