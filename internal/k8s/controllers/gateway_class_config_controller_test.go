package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

var (
	errExpected     = errors.New("expected")
	classConfigName = types.NamespacedName{
		Name:      "config",
		Namespace: "default",
	}
)

func TestGatewayClassConfigSetup(t *testing.T) {
	require.Error(t, (&GatewayClassConfigReconciler{}).SetupWithManager(nil))
}

func TestGatewayClassConfig(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		err           error
		result        reconcile.Result
		expectationCB func(client *mocks.MockClient)
	}{{
		name: "get-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient) {
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(nil, errExpected)
		},
	}, {
		name: "deleted",
		expectationCB: func(client *mocks.MockClient) {
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(nil, nil)
		},
	}, {
		name: "deleting-in-use-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient) {
			now := meta.Now()
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(&apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
			}, nil)
			client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(false, errExpected)
		},
	}, {
		name:   "deleting-in-use",
		result: ctrl.Result{RequeueAfter: 10 * time.Second},
		expectationCB: func(client *mocks.MockClient) {
			now := meta.Now()
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(&apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
			}, nil)
			client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(true, nil)
		},
	}, {
		name: "deleting-finalizer-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient) {
			now := meta.Now()
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(&apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
			}, nil)
			client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(false, nil)
			client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(false, errExpected)
		},
	}, {
		name: "deleting",
		expectationCB: func(client *mocks.MockClient) {
			now := meta.Now()
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(&apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					DeletionTimestamp: &now,
				},
			}, nil)
			client.EXPECT().GatewayClassConfigInUse(gomock.Any(), gomock.Any()).Return(false, nil)
			client.EXPECT().RemoveFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(true, nil)
		},
	}, {
		name: "create-finalizer-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient) {
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(false, errExpected)
		},
	}, {
		name: "create",
		expectationCB: func(client *mocks.MockClient) {
			client.EXPECT().GetGatewayClassConfig(gomock.Any(), classConfigName).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().EnsureFinalizer(gomock.Any(), gomock.Any(), gatewayClassConfigFinalizer).Return(true, nil)
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mocks.NewMockClient(ctrl)
			if test.expectationCB != nil {
				test.expectationCB(client)
			}

			controller := &GatewayClassConfigReconciler{
				Client: client,
				Log:    hclog.NewNullLogger(),
			}
			result, err := controller.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: classConfigName,
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
