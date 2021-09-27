package utils

import (
	"context"
	"fmt"
	"os"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

// WriteSecretCertFile retrieves a consul CA file stored in a K8s secret
func WriteSecretCertFile(restConfig *rest.Config, secret, file, namespace string) error {
	k8sClient, err := client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("failed to get k8s client: %w", err)
	}
	found := &core.Secret{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      secret,
	}, found)
	if err != nil {
		return fmt.Errorf("unable to pull Consul CA cert from secret: %w", err)
	}
	cert := found.Data[core.TLSCertKey]
	if err := os.WriteFile(file, cert, 0444); err != nil {
		return fmt.Errorf("unable to write CA cert: %w", err)
	}
	return nil
}
