package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/mocks"
	"github.com/hashicorp/go-hclog"
)

var (
	httpRouteName = types.NamespacedName{
		Name:      "http-route",
		Namespace: "default",
	}
)

func TestHTTPRouteSetup(t *testing.T) {
	require.Error(t, (&HTTPRouteReconciler{}).SetupWithManager(nil))
}

func TestHTTPRoute(t *testing.T) {
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
			client.EXPECT().GetHTTPRoute(gomock.Any(), httpRouteName).Return(nil, errExpected)
		},
	}, {
		name: "deleted",
		expectationCB: func(client *mocks.MockClient, reconciler *reconcilerMocks.MockReconcileManager) {
			client.EXPECT().GetHTTPRoute(gomock.Any(), httpRouteName).Return(nil, nil)
			reconciler.EXPECT().DeleteHTTPRoute(gomock.Any(), httpRouteName)
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

			controller := &HTTPRouteReconciler{
				Context:        context.Background(),
				Client:         client,
				Log:            hclog.NewNullLogger(),
				ControllerName: mockControllerName,
				Manager:        reconciler,
			}
			result, err := controller.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: httpRouteName,
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

func TestHTTPRouteReferenceGrantToRouteRequests(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	serviceNamespace := gwv1alpha2.Namespace("namespace3")

	backendObjRef := gwv1alpha2.BackendObjectReference{
		Name:      gwv1alpha2.ObjectName("service"),
		Namespace: &serviceNamespace,
	}

	httpRouteSpec := gwv1alpha2.HTTPRouteSpec{
		Rules: []gwv1alpha2.HTTPRouteRule{{
			BackendRefs: []gwv1alpha2.HTTPBackendRef{{
				BackendRef: gwv1alpha2.BackendRef{
					BackendObjectReference: backendObjRef,
				},
			}},
		}},
	}

	tcpRouteSpec := gwv1alpha2.TCPRouteSpec{
		Rules: []gwv1alpha2.TCPRouteRule{{
			BackendRefs: []gwv1alpha2.BackendRef{{
				BackendObjectReference: backendObjRef,
			}},
		}},
	}

	refGrant := gwv1alpha2.ReferenceGrant{
		TypeMeta:   metav1.TypeMeta{Kind: "ReferenceGrant"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "namespace3"},
		Spec: gwv1alpha2.ReferenceGrantSpec{
			From: []gwv1alpha2.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gwv1alpha2.ReferenceGrantTo{{
				Kind: "Service",
			}},
		},
	}

	client := gatewayclient.NewTestClient(
		nil,
		&gwv1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httproute",
				Namespace: "namespace1",
			},
			Spec: httpRouteSpec,
		},
		&gwv1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httproute",
				Namespace: "namespace2",
			},
			Spec: httpRouteSpec,
		},
		&gwv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tcproute",
				Namespace: "namespace1",
			},
			Spec: tcpRouteSpec,
		},
		&refGrant,
	)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconcilerMocks.NewMockReconcileManager(ctrl),
	}

	requests := controller.referenceGrantToRouteRequests(&refGrant)

	require.Equal(t, []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      "httproute",
			Namespace: "namespace1",
		},
	}}, requests)
}

func TestHTTPRouteReferencePolicyToRouteRequests(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	serviceNamespace := gwv1alpha2.Namespace("namespace3")

	backendObjRef := gwv1alpha2.BackendObjectReference{
		Name:      gwv1alpha2.ObjectName("service"),
		Namespace: &serviceNamespace,
	}

	httpRouteSpec := gwv1alpha2.HTTPRouteSpec{
		Rules: []gwv1alpha2.HTTPRouteRule{{
			BackendRefs: []gwv1alpha2.HTTPBackendRef{{
				BackendRef: gwv1alpha2.BackendRef{
					BackendObjectReference: backendObjRef,
				},
			}},
		}},
	}

	tcpRouteSpec := gwv1alpha2.TCPRouteSpec{
		Rules: []gwv1alpha2.TCPRouteRule{{
			BackendRefs: []gwv1alpha2.BackendRef{{
				BackendObjectReference: backendObjRef,
			}},
		}},
	}

	refPolicy := gwv1alpha2.ReferencePolicy{
		TypeMeta:   metav1.TypeMeta{Kind: "ReferencePolicy"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "namespace3"},
		Spec: gwv1alpha2.ReferenceGrantSpec{
			From: []gwv1alpha2.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gwv1alpha2.ReferenceGrantTo{{
				Kind: "Service",
			}},
		},
	}

	client := gatewayclient.NewTestClient(
		nil,
		&gwv1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httproute",
				Namespace: "namespace1",
			},
			Spec: httpRouteSpec,
		},
		&gwv1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httproute",
				Namespace: "namespace2",
			},
			Spec: httpRouteSpec,
		},
		&gwv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tcproute",
				Namespace: "namespace1",
			},
			Spec: tcpRouteSpec,
		},
		&refPolicy,
	)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconcilerMocks.NewMockReconcileManager(ctrl),
	}

	requests := controller.referencePolicyToRouteRequests(&refPolicy)

	require.Equal(t, []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      "httproute",
			Namespace: "namespace1",
		},
	}}, requests)
}
