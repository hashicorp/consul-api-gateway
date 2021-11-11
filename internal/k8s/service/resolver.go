package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

//go:generate mockgen -source ./resolver.go -destination ./mocks/resolver.go -package mocks BackendResolver

type ResolvedReferenceType int

type ServiceResolutionErrorType int

const (
	K8sServiceResolutionErrorType ServiceResolutionErrorType = iota
	ConsulServiceResolutionErrorType
	GenericResolutionErrorType
	NoResolutionErrorType
)

type ResolutionError struct {
	inner  string
	remote ServiceResolutionErrorType
}

func NewResolutionError(inner string) ResolutionError {
	return ResolutionError{inner, GenericResolutionErrorType}
}

func NewK8sResolutionError(inner string) ResolutionError {
	return ResolutionError{inner, K8sServiceResolutionErrorType}
}

func NewConsulResolutionError(inner string) ResolutionError {
	return ResolutionError{inner, ConsulServiceResolutionErrorType}
}

func (r ResolutionError) Error() string {
	return r.inner
}

type ResolutionErrors struct {
	k8sErrors     []ResolutionError
	consulErrors  []ResolutionError
	genericErrors []ResolutionError
}

func NewResolutionErrors() *ResolutionErrors {
	return &ResolutionErrors{}
}

func (r *ResolutionErrors) Add(err ResolutionError) {
	switch err.remote {
	case K8sServiceResolutionErrorType:
		r.k8sErrors = append(r.k8sErrors, err)
	case ConsulServiceResolutionErrorType:
		r.consulErrors = append(r.consulErrors, err)
	default:
		r.genericErrors = append(r.genericErrors, err)
	}
}

func (r *ResolutionErrors) String() string {
	errs := []string{}
	if len(r.k8sErrors) > 0 {
		k8sErrs := "k8s: "
		for i, err := range r.k8sErrors {
			if i != 0 {
				k8sErrs += ", "
			}
			k8sErrs += err.Error()
		}
		errs = append(errs, k8sErrs)
	}

	if len(r.consulErrors) > 0 {
		consulErrs := "consul: "
		for i, err := range r.consulErrors {
			if i != 0 {
				consulErrs += ", "
			}
			consulErrs += err.Error()
		}
		errs = append(errs, consulErrs)
	}

	if len(r.genericErrors) > 0 {
		genericErrs := "k8s: "
		for i, err := range r.genericErrors {
			if i != 0 {
				genericErrs += ", "
			}
			genericErrs += err.Error()
		}
		errs = append(errs, genericErrs)
	}

	return strings.Join(errs, "; ")
}

func (r *ResolutionErrors) Flatten() (ServiceResolutionErrorType, error) {
	if r.Empty() {
		return NoResolutionErrorType, nil
	}

	if len(r.genericErrors) != 0 || (len(r.consulErrors) != 0 && len(r.k8sErrors) != 0) {
		return GenericResolutionErrorType, errors.New(r.String())
	}

	if len(r.consulErrors) != 0 {
		return ConsulServiceResolutionErrorType, errors.New(r.String())
	}

	return K8sServiceResolutionErrorType, errors.New(r.String())
}

func (r *ResolutionErrors) Empty() bool {
	return len(r.k8sErrors) == 0 && len(r.consulErrors) == 0 && len(r.genericErrors) == 0
}

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

type BackendResolver interface {
	Resolve(ctx context.Context, ref gw.BackendObjectReference) (*ResolvedReference, error)
}

type backendResolver struct {
	namespace string
	client    gatewayclient.Client
	consul    *api.Client
	logger    hclog.Logger
}

func NewBackendResolver(logger hclog.Logger, namespace string, client gatewayclient.Client, consul *api.Client) BackendResolver {
	return &backendResolver{
		namespace: namespace,
		client:    client,
		consul:    consul,
		logger:    logger,
	}
}

func (r *backendResolver) Resolve(ctx context.Context, ref gw.BackendObjectReference) (*ResolvedReference, error) {
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
	namespacedName := types.NamespacedName{Name: string(ref.Name), Namespace: namespace}

	switch {
	case group == corev1.GroupName && kind == "Service":
		if ref.Port == nil {
			return nil, NewK8sResolutionError("service port must not be empty")
		}
		return r.consulServiceForK8SService(ctx, namespacedName)
	default:
		return nil, NewResolutionError("unsupported reference type")
	}
}

func (r *backendResolver) consulServiceForK8SService(ctx context.Context, namespacedName types.NamespacedName) (*ResolvedReference, error) {
	var err error
	var resolved *ResolvedReference

	service, err := r.client.GetService(ctx, namespacedName)
	if err != nil {
		r.logger.Error("error resolving kubernetes service", "error", err)
		return nil, err
	}
	if service == nil {
		r.logger.Warn("kubernetes service not found")
		return nil, NewK8sResolutionError("service not found")
	}

	// we do an inner retry since consul may take some time to sync
	err = backoff.Retry(func() error {
		r.logger.Trace("attempting to resolve global catalog service")
		resolved, err = r.findGlobalCatalogService(service)
		if err != nil {
			r.logger.Trace("error resolving global catalog reference", "error", err)
			return err
		}
		if resolved == nil {
			return NewConsulResolutionError("consul service not found")
		}
		return nil
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
	if err != nil {
		r.logger.Error("could not resolve consul service", "error", err)
		return nil, err
	}
	return resolved, nil
}

func validateAgentConsulReference(services map[string]*api.AgentService, object client.Object) (*ResolvedReference, error) {
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
			return nil,
				NewConsulResolutionError(fmt.Sprintf(
					"must have a single service map to a kubernetes service, found - (%q, %q) and (%q, %q)",
					serviceNamespace, serviceName, service.Namespace, service.Service,
				))
		}
	}
	return NewConsulServiceReference(object, &ConsulService{
		Name:      serviceName,
		Namespace: serviceNamespace,
	}), nil
}

// this acts as a brute-force mechanism for resolving a consul service if we can't find it registered
// in our local agent -- it checks all services based on their node with the same filtering mechanism
// we use in filtering the agent endpoint
func (r *backendResolver) findGlobalCatalogService(service *corev1.Service) (*ResolvedReference, error) {
	nodes, _, err := r.consul.Catalog().Nodes(nil)
	if err != nil {
		r.logger.Trace("error retrieving nodes", "error", err)
		return nil, err
	}
	filter := fmt.Sprintf(`Meta[%q] == %q and Meta[%q] == %q and Kind != "connect-proxy"`, MetaKeyKubeServiceName, service.Name, MetaKeyKubeNS, service.Namespace)
	for _, node := range nodes {
		nodeWithServices, _, err := r.consul.Catalog().Node(node.Node, &api.QueryOptions{
			Filter: filter,
		})
		if err != nil {
			r.logger.Trace("error retrieving node services", "error", err, "node", node.Node)
			return nil, err
		}
		if len(nodeWithServices.Services) == 0 {
			continue
		}
		resolved, err := validateAgentConsulReference(nodeWithServices.Services, service)
		if err != nil {
			r.logger.Trace("error validating node services", "error", err, "node", node.Node)
			return nil, err
		}
		if resolved != nil {
			return resolved, nil
		}
	}
	return nil, nil
}
