package gatewayclient

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func NewTestClient(list client.ObjectList, objects ...client.Object) Client {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gw.AddToScheme(scheme))
	apigwv1alpha1.RegisterTypes(scheme)

	builder := fake.
		NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...)

	if list != nil {
		builder = builder.WithLists(list)
	}

	return New(builder.Build(), scheme, "")
}
