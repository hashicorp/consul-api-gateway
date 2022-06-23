package gatewayclient

import (
	"context"
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGetGateway(t *testing.T) {
	t.Parallel()

	gatewayclient := NewTestClient(nil, &gateway.Gateway{
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

	gatewayclient := NewTestClient(nil, &apigwv1alpha1.GatewayClassConfig{
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

	gatewayclient := NewTestClient(nil, &gateway.HTTPRoute{
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

func TestGetHTTPRoutesInNamespace(t *testing.T) {
	t.Parallel()

	gatewayclient := NewTestClient(nil, &gateway.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Name:      "httproute",
			Namespace: "namespace1",
		},
	})

	routes, err := gatewayclient.GetHTTPRoutesInNamespace(context.Background(), "namespace1")
	require.NoError(t, err)
	require.Equal(t, len(routes), 1)

	routes, err = gatewayclient.GetHTTPRoutesInNamespace(context.Background(), "namespace2")
	require.NoError(t, err)
	require.Equal(t, len(routes), 0)
}

func TestGetGatewayClass(t *testing.T) {
	t.Parallel()

	gatewayclient := NewTestClient(nil, &gateway.GatewayClass{
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

	gatewayclient := NewTestClient(nil, &apps.Deployment{
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

	gatewayclient := NewTestClient(nil, &gateway.GatewayClass{
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

	gatewayclient := NewTestClient(nil, &gateway.GatewayClass{
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

func TestGatewayClassesUsingConfig(t *testing.T) {
	t.Parallel()

	gatewayClient := NewTestClient(&gateway.GatewayClassList{
		Items: []gateway.GatewayClass{
			{
				ObjectMeta: meta.ObjectMeta{Name: "gatewayclass1"},
				Spec: gateway.GatewayClassSpec{
					ParametersRef: &gateway.ParametersReference{
						Group: apigwv1alpha1.Group,
						Kind:  apigwv1alpha1.GatewayClassConfigKind,
						Name:  "gatewayclassconfig",
					},
				},
			},
			{
				ObjectMeta: meta.ObjectMeta{Name: "gatewayclass2"},
				Spec: gateway.GatewayClassSpec{
					ParametersRef: &gateway.ParametersReference{
						Group: apigwv1alpha1.Group,
						Kind:  apigwv1alpha1.GatewayClassConfigKind,
						Name:  "gatewayclassconfig",
					},
				},
			},
		},
	})

	using, err := gatewayClient.GatewayClassesUsingConfig(context.Background(), &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclassconfig",
		},
	})
	require.NoError(t, err)
	require.Len(t, using.Items, 2)
	assert.Equal(t, "gatewayclass1", using.Items[0].Name)
	assert.Equal(t, "gatewayclass2", using.Items[1].Name)

	using, err = gatewayClient.GatewayClassesUsingConfig(context.Background(), &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: "othergatewayclassconfig",
		},
	})
	require.NoError(t, err)
	assert.Empty(t, using.Items)
}

func TestGatewayClassConfigInUse(t *testing.T) {
	t.Parallel()

	gatewayclient := NewTestClient(&gateway.GatewayClassList{
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

	gatewayclient := NewTestClient(&gateway.GatewayList{
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
func TestPodsWithLabelsNoItems(t *testing.T) {
	gatewayclient := NewTestClient(nil)

	pods, err := gatewayclient.PodsWithLabels(context.Background(), map[string]string{
		"label": "test",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(pods))
}

func TestPodsWithLabelsOneItem(t *testing.T) {
	labels := map[string]string{
		"label": "test",
	}
	gatewayclient := NewTestClient(&core.PodList{
		Items: []core.Pod{{
			ObjectMeta: meta.ObjectMeta{
				Labels: labels,
			},
		}},
	})

	pod, err := gatewayclient.PodsWithLabels(context.Background(), labels)
	require.NoError(t, err)
	require.NotNil(t, pod)
}

func TestPodsWithLabelsMultipleItems(t *testing.T) {
	labels := map[string]string{
		"label": "test",
	}
	now := meta.Now()
	gatewayclient := NewTestClient(&core.PodList{
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

	pods, err := gatewayclient.PodsWithLabels(context.Background(), labels)
	require.NoError(t, err)
	require.NotNil(t, pods)
	require.Equal(t, 2, len(pods))
}
