package gatewayclient

import (
	"context"
	"testing"

	apps "k8s.io/api/apps/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestGetGateway(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(&gateway.Gateway{
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

	gatewayclient := newTestClient(&apigwv1alpha1.GatewayClassConfig{
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

	gatewayclient := newTestClient(&gateway.HTTPRoute{
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

	gatewayclient := newTestClient(&gateway.GatewayClass{
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

func TestGatewayClassForGateway(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclass",
		},
	})

	gatewayclass, err := gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "nogatewayclass",
		},
	})
	require.Error(t, err)
	require.Nil(t, gatewayclass)

	gatewayclass, err = gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, gatewayclass)
}

func TestDeploymentForGateway(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(&apps.Deployment{
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

	gatewayclient := newTestClient(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "gatewayclass",
		},
	})
	gatewayclass, err := gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 0)

	gatewayclient.EnsureFinalizer(context.Background(), gatewayclass, "finalizer")
	gatewayclass, err = gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 1)
	require.Equal(t, "finalizer", gatewayclass.Finalizers[0])

	// make sure it only adds it once
	gatewayclient.EnsureFinalizer(context.Background(), gatewayclass, "finalizer")
	gatewayclass, err = gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 1)
	require.Equal(t, "finalizer", gatewayclass.Finalizers[0])
}

func TestRemoveinalizer(t *testing.T) {
	t.Parallel()

	gatewayclient := newTestClient(&gateway.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name:       "gatewayclass",
			Finalizers: []string{"finalizer", "other"},
		},
	})
	gatewayclass, err := gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 2)

	gatewayclient.RemoveFinalizer(context.Background(), gatewayclass, "other")
	gatewayclass, err = gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 1)
	require.Equal(t, "finalizer", gatewayclass.Finalizers[0])

	gatewayclient.RemoveFinalizer(context.Background(), gatewayclass, "finalizer")
	gatewayclass, err = gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 0)

	// make sure it handles non-existent finalizers
	gatewayclient.RemoveFinalizer(context.Background(), gatewayclass, "nonexistent")
	gatewayclass, err = gatewayclient.GatewayClassForGateway(context.Background(), &gateway.Gateway{
		Spec: gateway.GatewaySpec{
			GatewayClassName: "gatewayclass",
		},
	})
	require.NoError(t, err)
	require.Len(t, gatewayclass.Finalizers, 0)
}

func newTestClient(objects ...client.Object) Client {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gateway.AddToScheme(scheme))
	apigwv1alpha1.RegisterTypes(scheme)

	builder := fake.
		NewClientBuilder().
		WithScheme(scheme)
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}

	return New(builder.Build(), scheme)
}
