package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/mocks"
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

func TestHTTPRouteServiceToRouteRequests(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "echo-1"},
	}

	backendNS := gw.Namespace("namespace-1")

	client := gatewayclient.NewTestClient(
		nil,
		// Include one route that references the Service name without the namespace,
		// meaning that the namespace is implicitly the route's namespace
		&gw.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "route-1"},
			Spec: gw.HTTPRouteSpec{
				Rules: []gw.HTTPRouteRule{{
					BackendRefs: []gw.HTTPBackendRef{
						{
							BackendRef: gw.BackendRef{BackendObjectReference: gw.BackendObjectReference{Name: "echo-1"}},
						},
						{
							BackendRef: gw.BackendRef{BackendObjectReference: gw.BackendObjectReference{Name: "echo-2"}},
						},
					},
				}},
			},
		},
		// Include one route in a different namespace that references the Service by
		// explicit namespace + name
		&gw.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-2", Name: "route-2"},
			Spec: gw.HTTPRouteSpec{
				Rules: []gw.HTTPRouteRule{{
					BackendRefs: []gw.HTTPBackendRef{{
						BackendRef: gw.BackendRef{
							BackendObjectReference: gw.BackendObjectReference{
								Namespace: &backendNS,
								Name:      "echo-1",
							},
						},
					}},
				}},
			},
		},
		// Include one route in the same namespace that does not reference the Service
		&gw.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "route-3"},
			Spec: gw.HTTPRouteSpec{
				Rules: []gw.HTTPRouteRule{{
					BackendRefs: []gw.HTTPBackendRef{},
				}},
			},
		},
		// Include one route in a different namespace that does not reference the Service
		&gw.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-2", Name: "route-4"},
			Spec: gw.HTTPRouteSpec{
				Rules: []gw.HTTPRouteRule{{
					BackendRefs: []gw.HTTPBackendRef{},
				}},
			},
		},
	)

	controller := &HTTPRouteReconciler{
		Client:         client,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconcilerMocks.NewMockReconcileManager(ctrl),
	}

	requests := controller.serviceToRouteRequests(svc)
	require.Len(t, requests, 2)
	assert.Equal(t, "namespace-1/route-1", requests[0].String())
	assert.Equal(t, "namespace-2/route-2", requests[1].String())
}

func TestHTTPRouteReferencePolicyToRouteRequests(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	serviceNamespace := gw.Namespace("namespace3")

	backendObjRef := gw.BackendObjectReference{
		Name:      gw.ObjectName("service"),
		Namespace: &serviceNamespace,
	}

	httpRouteSpec := gw.HTTPRouteSpec{
		Rules: []gw.HTTPRouteRule{{
			BackendRefs: []gw.HTTPBackendRef{{
				BackendRef: gw.BackendRef{
					BackendObjectReference: backendObjRef,
				},
			}},
		}},
	}

	tcpRouteSpec := gw.TCPRouteSpec{
		Rules: []gw.TCPRouteRule{{
			BackendRefs: []gw.BackendRef{{
				BackendObjectReference: backendObjRef,
			}},
		}},
	}

	refPolicy := gw.ReferencePolicy{
		TypeMeta:   metav1.TypeMeta{Kind: "ReferencePolicy"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "namespace3"},
		Spec: gw.ReferencePolicySpec{
			From: []gw.ReferencePolicyFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gw.ReferencePolicyTo{{
				Kind: "Service",
			}},
		},
	}

	client := gatewayclient.NewTestClient(
		nil,
		&gw.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httproute",
				Namespace: "namespace1",
			},
			Spec: httpRouteSpec,
		},
		&gw.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httproute",
				Namespace: "namespace2",
			},
			Spec: httpRouteSpec,
		},
		&gw.TCPRoute{
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
