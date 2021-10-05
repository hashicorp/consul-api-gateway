package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/consul/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	routeReference resolvedReferenceType = iota
	consulServiceReference
)

type routeRule struct {
	httpRule *gw.HTTPRouteRule
}

type consulService struct {
	namespace string
	name      string
}

type resolvedReference struct {
	referenceType resolvedReferenceType
	ref           *backendRef
	object        client.Object
	consulService *consulService
}

type routeRuleReferenceMap map[routeRule][]resolvedReference

func newConsulServiceReference(object client.Object) *resolvedReference {
	return &resolvedReference{
		referenceType: consulServiceReference,
		object:        object,
		ref:           &backendRef{},
	}
}

func (r *resolvedReference) SetConsul(service *consulService) *resolvedReference {
	r.consulService = service
	return r
}

func (r *resolvedReference) Item() client.Object {
	return r.object
}

type backendRef struct {
	httpRef *gw.HTTPBackendRef
}

func (b *backendRef) Set(ref interface{}) {
	switch backendRef := ref.(type) {
	case *gw.HTTPBackendRef:
		b.httpRef = backendRef
	}
}

type backendResolver struct {
	namespace string
	client    gatewayclient.Client
	consul    *api.Client
}

func newBackendResolver(namespace string, client gatewayclient.Client, consul *api.Client) *backendResolver {
	return &backendResolver{
		namespace: namespace,
		client:    client,
		consul:    consul,
	}
}

func (r *backendResolver) resolveBackendReference(ctx context.Context, ref gw.BackendObjectReference) (*resolvedReference, error) {
	group := corev1.GroupName
	kind := "Service"
	namespace := r.namespace
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

	switch {
	case group == corev1.GroupName && kind == "Service":
		if ref.Port == nil {
			return nil, ErrEmptyPort
		}
		return r.consulServiceForK8SService(ctx, namespacedName)
	case group == apigwv1alpha1.Group && kind == apigwv1alpha1.MeshServiceKind:
		return r.consulServiceForMeshService(ctx, namespacedName)
	default:
		return nil, ErrUnsupportedReference
	}
}

func (r *backendResolver) consulServiceForK8SService(ctx context.Context, namespacedName types.NamespacedName) (*resolvedReference, error) {
	var err error
	var resolved *resolvedReference

	service, err := r.client.GetService(ctx, namespacedName)
	if err != nil {
		return nil, fmt.Errorf("error resolving reference: %w", err)
	}
	if service == nil {
		return nil, ErrNotResolved
	}

	// we do an inner retry since consul may take some time to sync
	err = backoff.Retry(func() error {
		services, err := r.consul.Agent().ServicesWithFilter(fmt.Sprintf(`Meta[%q] == %q and Meta[%q] == %q and Kind != "connect-proxy"`, MetaKeyKubeServiceName, service.Name, MetaKeyKubeNS, service.Namespace))
		if err != nil {
			return err
		}
		resolved, err = validateConsulReference(services, service)
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func (r *backendResolver) consulServiceForMeshService(ctx context.Context, namespacedName types.NamespacedName) (*resolvedReference, error) {
	var err error
	var resolved *resolvedReference

	service, err := r.client.GetMeshService(ctx, namespacedName)
	if err != nil {
		return nil, fmt.Errorf("error resolving reference: %w", err)
	}
	if service == nil {
		return nil, ErrNotResolved
	}

	filter := fmt.Sprintf("Service == %q", service.Spec.Name)
	options := &api.QueryOptions{}
	if service.Spec.Namespace != "" {
		options.Namespace = service.Spec.Namespace
	}

	// we do an inner retry since consul may take some time to sync
	err = backoff.Retry(func() error {
		services, err := r.consul.Agent().ServicesWithFilterOpts(filter, options)
		if err != nil {
			return err
		}
		resolved, err = validateConsulReference(services, service)
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func validateConsulReference(services map[string]*api.AgentService, object client.Object) (*resolvedReference, error) {
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
	return newConsulServiceReference(object).SetConsul(&consulService{
		name:      serviceName,
		namespace: serviceNamespace,
	}), nil
}
