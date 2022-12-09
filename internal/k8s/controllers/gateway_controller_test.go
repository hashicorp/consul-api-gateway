// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
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

func TestGatewayReferenceGrantToGatewayRequests(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretNamespace := gwv1beta1.Namespace("namespace3")

	gatewayTLSConfig := gwv1beta1.GatewayTLSConfig{
		CertificateRefs: []gwv1beta1.SecretObjectReference{{
			Name:      gwv1beta1.ObjectName("secret"),
			Namespace: &secretNamespace,
		}},
	}

	gatewaySpec := gwv1beta1.GatewaySpec{
		Listeners: []gwv1beta1.Listener{{
			TLS: &gatewayTLSConfig,
		}},
	}

	refGrant := gwv1alpha2.ReferenceGrant{
		TypeMeta:   metav1.TypeMeta{Kind: "ReferenceGrant"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "namespace3"},
		Spec: gwv1alpha2.ReferenceGrantSpec{
			From: []gwv1alpha2.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "Gateway",
				Namespace: "namespace1",
			}},
			To: []gwv1alpha2.ReferenceGrantTo{{
				Kind: "Secret",
			}},
		},
	}

	client := gatewayclient.NewTestClient(
		nil,
		&gwv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway",
				Namespace: "namespace1",
			},
			Spec: gatewaySpec,
		},
		&gwv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway",
				Namespace: "namespace2",
			},
			Spec: gatewaySpec,
		},
		&refGrant,
	)

	controller := &GatewayReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconcilerMocks.NewMockReconcileManager(ctrl),
	}

	requests := controller.referenceGrantToGatewayRequests(&refGrant)

	assert.Equal(t, []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      "gateway",
			Namespace: "namespace1",
		},
	}}, requests)
}

func TestGatewayReferencePolicyToGatewayRequests(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretNamespace := gwv1beta1.Namespace("namespace3")

	gatewayTLSConfig := gwv1beta1.GatewayTLSConfig{
		CertificateRefs: []gwv1beta1.SecretObjectReference{{
			Name:      gwv1beta1.ObjectName("secret"),
			Namespace: &secretNamespace,
		}},
	}

	gatewaySpec := gwv1beta1.GatewaySpec{
		Listeners: []gwv1beta1.Listener{{
			TLS: &gatewayTLSConfig,
		}},
	}

	refPolicy := gwv1alpha2.ReferencePolicy{
		TypeMeta:   metav1.TypeMeta{Kind: "ReferencePolicy"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "namespace3"},
		Spec: gwv1alpha2.ReferenceGrantSpec{
			From: []gwv1alpha2.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "Gateway",
				Namespace: "namespace1",
			}},
			To: []gwv1alpha2.ReferenceGrantTo{{
				Kind: "Secret",
			}},
		},
	}

	client := gatewayclient.NewTestClient(
		nil,
		&gwv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway",
				Namespace: "namespace1",
			},
			Spec: gatewaySpec,
		},
		&gwv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway",
				Namespace: "namespace2",
			},
			Spec: gatewaySpec,
		},
		&refPolicy,
	)

	controller := &GatewayReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconcilerMocks.NewMockReconcileManager(ctrl),
	}

	requests := controller.referencePolicyToGatewayRequests(&refPolicy)

	assert.Equal(t, []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      "gateway",
			Namespace: "namespace1",
		},
	}}, requests)
}
