package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/k8s/reconciler/mocks"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestGatewaySetup(t *testing.T) {
	require.Error(t, (&GatewayReconciler{}).SetupWithManager(nil))
}

func TestPodToGatewayRequest(t *testing.T) {
	requests := podToGatewayRequest(&core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "default",
			Labels:    utils.LabelsForNamedGateway(gatewayName),
		},
	})
	require.Len(t, requests, 1)
	require.Equal(t, reconcile.Request{
		NamespacedName: gatewayName,
	}, requests[0])

	requests = podToGatewayRequest(&core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "default",
		},
	})
	require.Len(t, requests, 0)
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
	}, {
		name: "deployment-exists-no-pod",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(&apps.Deployment{}, nil)
			client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, gatewayclient.ErrPodNotCreated)
		},
	}, {
		name: "deployment-class-config-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(nil, errExpected)
		},
	}, {
		name: "deployment-controller-ownership-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(errExpected)
		},
	}, {
		name: "deployment-creation-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).Return(errExpected)
		},
	}, {
		name: "service-controller-ownership-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			loadBalancerType := core.ServiceTypeLoadBalancer
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ServiceType: &loadBalancerType,
				},
			}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(errExpected)
		},
	}, {
		name: "service-creation-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			loadBalancerType := core.ServiceTypeLoadBalancer
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ServiceType: &loadBalancerType,
				},
			}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateService(gomock.Any(), gomock.Any()).Return(errExpected)
		},
	}, {
		name: "pod-error",
		err:  errExpected,
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, errExpected)
		},
	}, {
		name: "pod-conditions-updated",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{}, nil)
			tracker.EXPECT().UpdateStatus(gatewayName, gomock.Any(), gomock.Any()).Return(true)
		},
	}, {
		name: "pod-conditions-not-updated",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager, tracker *reconcilerMocks.MockGatewayStatusTracker) {
			client.EXPECT().GetGateway(gomock.Any(), gatewayName).Return(&gateway.Gateway{}, nil)
			client.EXPECT().GatewayClassForGateway(gomock.Any(), gomock.Any()).Return(&gateway.GatewayClass{
				Spec: gateway.GatewayClassSpec{
					Controller: gateway.GatewayController(mockControllerName),
				},
			}, nil)
			reconciler.EXPECT().UpsertGateway(gomock.Any())
			client.EXPECT().DeploymentForGateway(gomock.Any(), gomock.Any()).Return(nil, nil)
			client.EXPECT().GatewayClassConfigForGatewayClass(gomock.Any(), gomock.Any()).Return(&apigwv1alpha1.GatewayClassConfig{}, nil)
			client.EXPECT().SetControllerOwnership(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).Return(nil)
			client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{}, nil)
			tracker.EXPECT().UpdateStatus(gatewayName, gomock.Any(), gomock.Any()).Return(false)
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
