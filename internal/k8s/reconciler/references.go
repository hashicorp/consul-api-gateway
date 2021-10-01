package reconciler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/consul/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func resolveBackendReference(ctx context.Context, client gatewayclient.Client, ref gw.BackendObjectReference, route *K8sRoute, consul *api.Client) (*resolvedReference, error) {
	group := corev1.GroupName
	kind := "Service"
	namespace := route.GetNamespace()
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	namespacedName := types.NamespacedName{Name: ref.Name, Namespace: namespace}

	var resolvedRef *resolvedReference

	switch {
	case group == corev1.GroupName && kind == "Service":
		if ref.Port == nil {
			return nil, ErrEmptyPort
		}
		svc, err := client.GetService(ctx, namespacedName)
		if err != nil {
			return nil, fmt.Errorf("error resolving reference: %w", err)
		}
		if svc == nil {
			return nil, ErrNotResolved
		}
		err = backoff.Retry(func() error {
			// we do an inner retry since consul may take some time to sync
			services, err := serviceInstancesForK8SServiceNameAndNamespace(svc.Name, svc.Namespace, consul)
			if err != nil {
				return fmt.Errorf("error resolving reference: %w", err)
			}
			resolvedRef, err = validateConsulReference(services)
			return err
		}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
		return resolvedRef, err
	case group == apigwv1alpha1.Group && kind == apigwv1alpha1.MeshServiceKind:
		svc, err := client.GetMeshService(ctx, namespacedName)
		if err != nil {
			return nil, fmt.Errorf("error resolving reference: %w", err)
		}
		if svc == nil {
			return nil, ErrNotResolved
		}
		err = backoff.Retry(func() error {
			services, err := consulServiceWithName(svc.Name, svc.Namespace, consul)
			if err != nil {
				return fmt.Errorf("error resolving reference: %w", err)
			}
			resolvedRef, err = validateConsulReference(services)
			return err
		}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
		return resolvedRef, err
	default:
		return nil, ErrUnsupportedReference
	}
}

func validateConsulReference(services map[string]*api.AgentService) (*resolvedReference, error) {
	if len(services) == 0 {
		return nil, ErrConsulNotResolved
	}
	serviceName := ""
	serviceNamespace := ""
	for _, service := range services {
		if serviceName == "" {
			serviceName = service.Service
		}
		if serviceNamespace == "" {
			serviceNamespace = service.Namespace
		}
		if service.Service != serviceName || service.Namespace != serviceNamespace {
			return nil, fmt.Errorf(
				"must have a single service map to a kubernetes service, found: (%q, %q) and (%q, %q): %w",
				serviceNamespace, serviceName, service.Namespace, service.Service, ErrConsulNotResolved,
			)
		}
	}
	return &resolvedReference{
		referenceType: consulServiceReference,
		consulService: &consulService{
			name:      serviceName,
			namespace: serviceNamespace,
		},
	}, nil
}

func serviceInstancesForK8SServiceNameAndNamespace(k8sServiceName, k8sServiceNamespace string, client *api.Client) (map[string]*api.AgentService, error) {
	log.Println(fmt.Sprintf(`Meta[%q] == %q and Meta[%q] == %q and Kind != "connect-proxy"`, MetaKeyKubeServiceName, k8sServiceName, MetaKeyKubeNS, k8sServiceNamespace))
	return client.Agent().ServicesWithFilter(
		fmt.Sprintf(`Meta[%q] == %q and Meta[%q] == %q and Kind != "connect-proxy"`, MetaKeyKubeServiceName, k8sServiceName, MetaKeyKubeNS, k8sServiceNamespace))
}

func consulServiceWithName(name, namespace string, client *api.Client) (map[string]*api.AgentService, error) {
	filter := fmt.Sprintf("Service == %q", name)
	options := &api.QueryOptions{}
	if namespace != "" {
		options.Namespace = namespace
	}
	return client.Agent().ServicesWithFilterOpts(filter, options)
}
