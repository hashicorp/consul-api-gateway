package controllers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	reconcilerMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/mocks"
	"github.com/hashicorp/go-hclog"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
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

	gatewayclient := NewTestClient(
		&gw.HTTPRouteList{
			Items: []gw.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "httproute",
						Namespace: "namespace1",
					},
					Spec: httpRouteSpec,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "httproute",
						Namespace: "namespace2",
					},
					Spec: httpRouteSpec,
				},
			},
		},
		&refPolicy,
	)

	controller := &HTTPRouteReconciler{
		Client:         gatewayclient,
		Log:            hclog.NewNullLogger(),
		ControllerName: mockControllerName,
		Manager:        reconcilerMocks.NewMockReconcileManager(ctrl),
	}

	requests := controller.referencePolicyToRouteRequests(&refPolicy)

	// FIXME: This should only request reconciliation for the one HTTPRoute in the
	// allowed namespace
	//
	// require.Equal(t, 1, len(requests))
	require.Equal(t, []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      "httproute",
			Namespace: "namespace1",
		},
	}}, requests)
}

// FIXME: this should be refactored into a test utility package
func NewTestClient(list client.ObjectList, objects ...client.Object) gatewayclient.Client {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gw.AddToScheme(scheme))
	apigwv1alpha1.RegisterTypes(scheme)

	builder := fake.
		NewClientBuilder().
		WithScheme(scheme)
	if list != nil {
		builder = builder.WithLists(list)
	}
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}

	return gatewayclient.New(builder.Build(), scheme, "")
}
