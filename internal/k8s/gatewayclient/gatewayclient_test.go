package gatewayclient

import (
	"context"
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/stretchr/testify/require"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGetGateway(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &gateway.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "namespace",
		},
	})

	gateway, err := gatewayclient.GetGateway(context.Background(), types.NamespacedName{
		Name:      "gateway",
		Namespace: "other",
	})
	require.NoError(t, err)
	require.Nil(t, gateway)

	gateway, err = gatewayclient.GetGateway(context.Background(), types.NamespacedName{
		Name:      "gateway",
		Namespace: "namespace",
	})
	require.NoError(t, err)
	require.NotNil(t, gateway)
}

func TestGetGatewayClassConfig(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclassconfig",
		},
	})

	gatewayclassconfig, err := gatewayclient.GetGatewayClassConfig(context.Background(), types.NamespacedName{
		Name: "nogatewayclassconfig",
	})
	require.NoError(t, err)
	require.Nil(t, gatewayclassconfig)

	gatewayclassconfig, err = gatewayclient.GetGatewayClassConfig(context.Background(), types.NamespacedName{
		Name: "gatewayclassconfig",
	})
	require.NoError(t, err)
	require.NotNil(t, gatewayclassconfig)
}

func TestGetHTTPRoute(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &gateway.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Name:      "httproute",
			Namespace: "namespace",
		},
	})

	route, err := gatewayclient.GetHTTPRoute(context.Background(), types.NamespacedName{
		Name:      "nohttproute",
		Namespace: "namespace",
	})
	require.NoError(t, err)
	require.Nil(t, route)

	route, err = gatewayclient.GetHTTPRoute(context.Background(), types.NamespacedName{
		Name:      "httproute",
		Namespace: "nonamespace",
	})
	require.NoError(t, err)
	require.Nil(t, route)

	route, err = gatewayclient.GetHTTPRoute(context.Background(), types.NamespacedName{
		Name:      "httproute",
		Namespace: "namespace",
	})
	require.NoError(t, err)
	require.NotNil(t, route)
}

func TestGetGatewayClass(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclass",
		},
	})

	gatewayclass, err := gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "nogatewayclass",
	})
	require.NoError(t, err)
	require.Nil(t, gatewayclass)

	gatewayclass, err = gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.NotNil(t, gatewayclass)
}

func TestDeploymentForGateway(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "namespace",
		},
	})

	deployment, err := gatewayclient.DeploymentForGateway(context.Background(), &gateway.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "notgateway",
			Namespace: "namespace",
		},
	})
	require.NoError(t, err)
	require.Nil(t, deployment)

	deployment, err = gatewayclient.DeploymentForGateway(context.Background(), &gateway.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "notnamespace",
		},
	})
	require.NoError(t, err)
	require.Nil(t, deployment)

	deployment, err = gatewayclient.DeploymentForGateway(context.Background(), &gateway.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "namespace",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, deployment)
}

func TestEnsureFinalizer(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclass",
		},
	})
	gatewayclass, err := gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 0)

	_, err = gatewayclient.EnsureFinalizer(context.Background(), gatewayclass, "finalizer")
	require.NoError(t, err)
	gatewayclass, err = gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 1)
	require.Equal(t, "finalizer", gatewayclass.Finalizers[0])

	// make sure it only adds it once
	_, err = gatewayclient.EnsureFinalizer(context.Background(), gatewayclass, "finalizer")
	require.NoError(t, err)
	gatewayclass, err = gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 1)
	require.Equal(t, "finalizer", gatewayclass.Finalizers[0])
}

func TestRemoveFinalizer(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(nil, &gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name:       "gatewayclass",
			Finalizers: []string{"finalizer", "other"},
		},
	})
	gatewayclass, err := gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 2)

	_, err = gatewayclient.RemoveFinalizer(context.Background(), gatewayclass, "other")
	require.NoError(t, err)
	gatewayclass, err = gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 1)
	require.Equal(t, "finalizer", gatewayclass.Finalizers[0])

	_, err = gatewayclient.RemoveFinalizer(context.Background(), gatewayclass, "finalizer")
	require.NoError(t, err)
	gatewayclass, err = gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 0)

	// make sure it handles non-existent finalizers
	_, err = gatewayclient.RemoveFinalizer(context.Background(), gatewayclass, "nonexistent")
	require.NoError(t, err)
	gatewayclass, err = gatewayclient.GetGatewayClass(context.Background(), types.NamespacedName{
		Name: "gatewayclass",
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 0)
}

func TestGatewayClassConfigInUse(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(&gateway.GatewayClassList{
		Items: []gateway.GatewayClass{{
			ObjectMeta: meta.ObjectMeta{
				Name: "gatewayclass",
			},
			Spec: gateway.GatewayClassSpec{
				ParametersRef: &gateway.ParametersReference{
					Group: apigwv1alpha1.Group,
					Kind:  apigwv1alpha1.GatewayClassConfigKind,
					Name:  "gatewayclassconfig",
				},
			},
		}}})

	used, err := gatewayclient.GatewayClassConfigInUse(context.Background(), &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclassconfig",
		},
	})
	require.NoError(t, err)
	require.True(t, used)

	used, err = gatewayclient.GatewayClassConfigInUse(context.Background(), &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: "nogatewayclassconfig",
		},
	})
	require.NoError(t, err)
	require.False(t, used)
}

func TestGatewayClassInUse(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(&gateway.GatewayList{
		Items: []gateway.Gateway{{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
			Spec: gateway.GatewaySpec{
				GatewayClassName: "gatewayclass",
			},
		}}})

	used, err := gatewayclient.GatewayClassInUse(context.Background(), &gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.True(t, used)

	used, err = gatewayclient.GatewayClassInUse(context.Background(), &gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "nogatewayclass",
		},
	})
	require.NoError(t, err)
	require.False(t, used)
}
func TestPodWithLabelsNoItems(t *testing.T) {
	gatewayclient := newTestClient(nil)

	pod, err := gatewayclient.PodWithLabels(context.Background(), map[string]string{
		"label": "test",
	})
	require.NoError(t, err)
	require.Nil(t, pod)
}

func TestPodWithLabelsOneItem(t *testing.T) {
	labels := map[string]string{
		"label": "test",
	}
	gatewayclient := newTestClient(&core.PodList{
		Items: []core.Pod{{
			ObjectMeta: meta.ObjectMeta{
				Labels: labels,
			},
		}},
	})

	pod, err := gatewayclient.PodWithLabels(context.Background(), labels)
	require.NoError(t, err)
	require.NotNil(t, pod)
}

func TestPodWithLabelsMultipleItems(t *testing.T) {
	labels := map[string]string{
		"label": "test",
	}
	now := meta.Now()
	gatewayclient := newTestClient(&core.PodList{
		Items: []core.Pod{{
			ObjectMeta: meta.ObjectMeta{
				Labels:            labels,
				DeletionTimestamp: &now,
				Name:              "one",
			},
		}, {
			ObjectMeta: meta.ObjectMeta{
				Labels: labels,
				Name:   "two",
			},
		}, {
			ObjectMeta: meta.ObjectMeta{
				Labels: labels,
				Name:   "three",
			},
		}},
	})

	pod, err := gatewayclient.PodWithLabels(context.Background(), labels)
	require.NoError(t, err)
	require.NotNil(t, pod)
}

func newTestClient(list client.ObjectList, objects ...client.Object) Client {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gateway.AddToScheme(scheme))
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

	return New(builder.Build(), scheme, "")
}
