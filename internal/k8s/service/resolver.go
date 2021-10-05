package service

import (
	"context"
	"errors"
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

type ResolvedReferenceType int

var (
	ErrEmptyPort            = errors.New("port cannot be empty with kubernetes service")
	ErrNotResolved          = errors.New("backend reference not found")
	ErrConsulNotResolved    = errors.New("consul service not found")
	ErrUnsupportedReference = errors.New("unsupported reference type")
)

const (
	HTTPRouteReference ResolvedReferenceType = iota
	ConsulServiceReference

	MetaKeyKubeServiceName = "k8s-service-name"
	MetaKeyKubeNS          = "k8s-namespace"
)

type ConsulService struct {
	Namespace string
	Name      string
}

type BackendReference struct {
	HTTPRef    *gw.HTTPBackendRef
	BackendRef *gw.BackendRef
}

func (b *BackendReference) Set(reference interface{}) {
	switch ref := reference.(type) {
	case *gw.HTTPBackendRef:
		b.HTTPRef = ref
	case *gw.BackendRef:
		b.BackendRef = ref
	}
}

type ResolvedReference struct {
	Type      ResolvedReferenceType
	Reference *BackendReference
	Consul    *ConsulService
	object    client.Object
}

func NewConsulServiceReference(object client.Object, consul *ConsulService) *ResolvedReference {
	return &ResolvedReference{
		Type:      ConsulServiceReference,
		Reference: &BackendReference{},
		Consul:    consul,
		object:    object,
	}
}

func (r *ResolvedReference) Item() client.Object {
	return r.object
}

type BackendResolver struct {
	namespace string
	client    gatewayclient.Client
	consul    *api.Client
}

func NewBackendResolver(namespace string, client gatewayclient.Client, consul *api.Client) *BackendResolver {
	return &BackendResolver{
		namespace: namespace,
		client:    client,
		consul:    consul,
	}
}

func (r *BackendResolver) Resolve(ctx context.Context, ref gw.BackendObjectReference) (*ResolvedReference, error) {
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

func (r *BackendResolver) consulServiceForK8SService(ctx context.Context, namespacedName types.NamespacedName) (*ResolvedReference, error) {
	var err error
	var resolved *ResolvedReference

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

func (r *BackendResolver) consulServiceForMeshService(ctx context.Context, namespacedName types.NamespacedName) (*ResolvedReference, error) {
	var err error
	var resolved *ResolvedReference

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

func validateConsulReference(services map[string]*api.AgentService, object client.Object) (*ResolvedReference, error) {
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
	return NewConsulServiceReference(object, &ConsulService{
		Name:      serviceName,
		Namespace: serviceNamespace,
	}), nil
}
