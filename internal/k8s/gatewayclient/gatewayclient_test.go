package gatewayclient

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestGatewayClient(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gateway.AddToScheme(scheme))
	apigwv1alpha1.RegisterTypes(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	gatewayclient := New(client, scheme)
	gateway, err := gatewayclient.GetGateway(context.Background(), types.NamespacedName{
		Name:      "gateway",
		Namespace: "namespace",
	})
	require.NoError(t, err)
	require.Nil(t, gateway)
}
