// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

//go:generate mockgen -source ./resolver.go -destination ./mocks/resolver.go -package mocks BackendResolver

type ResolvedReferenceType string

type ServiceResolutionErrorType string

const (
	K8sServiceResolutionErrorType    ServiceResolutionErrorType = ""
	BackendNotFoundErrorType         ServiceResolutionErrorType = "BackendNotFoundError"
	ConsulServiceResolutionErrorType ServiceResolutionErrorType = "ConsulServiceResolutionError"
	GenericResolutionErrorType       ServiceResolutionErrorType = "GenericResolutionError"
	InvalidKindErrorType             ServiceResolutionErrorType = "InvalidKindError"
	NoResolutionErrorType            ServiceResolutionErrorType = "NoResolutionError"
	RefNotPermittedErrorType         ServiceResolutionErrorType = "RefNotPermittedError"
)

var errorTypePrefixMap = map[ServiceResolutionErrorType]string{
	K8sServiceResolutionErrorType:    "k8s: ",
	ConsulServiceResolutionErrorType: "consul: ",
}

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

func NewBackendNotFoundError(inner string) ResolutionError {
	return ResolutionError{inner, BackendNotFoundErrorType}
}

func NewConsulResolutionError(inner string) ResolutionError {
	return ResolutionError{inner, ConsulServiceResolutionErrorType}
}

func NewInvalidKindError(inner string) ResolutionError {
	return ResolutionError{inner, InvalidKindErrorType}
}

func NewRefNotPermittedError(inner string) ResolutionError {
	return ResolutionError{inner, RefNotPermittedErrorType}
}

func getErrorTypePrefix(errType ServiceResolutionErrorType) string {
	return errorTypePrefixMap[errType]
}

func (r ResolutionError) Error() string {
	return r.inner
}

type ResolutionErrors struct {
	errors map[ServiceResolutionErrorType][]ResolutionError
}

func NewResolutionErrors() *ResolutionErrors {
	return &ResolutionErrors{errors: make(map[ServiceResolutionErrorType][]ResolutionError)}
}

func (r *ResolutionErrors) Add(err ResolutionError) {
	r.errors[err.remote] = append(r.errors[err.remote], err)
}

func (r *ResolutionErrors) String() string {
	errs := []string{}

	for errType, errors := range r.errors {
		if len(errors) > 0 {
			errorString := getErrorTypePrefix(errType)
			for i, err := range errors {
				if i != 0 {
					errorString += ", "
				}
				errorString += err.Error()
			}
			errs = append(errs, errorString)
		}
	}
	return strings.Join(errs, "; ")
}

func (r *ResolutionErrors) Flatten() (ServiceResolutionErrorType, error) {
	if r.Empty() {
		return NoResolutionErrorType, nil
	}

	//return generic error if there are multiple errors types, or if generic errors exist
	if len(r.errors[GenericResolutionErrorType]) != 0 || (len(r.errors) > 1) {
		return GenericResolutionErrorType, errors.New(r.String())
	}

	// we only have at most one error type at this point, so return the error type of the first set of errors we find
	for errType := range r.errors {
		return errType, errors.New(r.String())
	}

	//shouldn't be possible to get here
	return GenericResolutionErrorType, errors.New(r.String())
}

func (r *ResolutionErrors) Empty() bool {
	return len(r.errors) == 0
}

func (r *ResolutionErrors) UnmarshalJSON(b []byte) error {
	errs := make(map[ServiceResolutionErrorType][]ResolutionError)
	if err := json.Unmarshal(b, &errs); err != nil {
		return err
	}
	r.errors = errs
	return nil
}

func (r ResolutionErrors) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.errors)
}

const (
	HTTPRouteReference     ResolvedReferenceType = "HTTPRoute"
	ConsulServiceReference ResolvedReferenceType = "ConsulService"

	MetaKeyKubeServiceName = "k8s-service-name"
	MetaKeyKubeNS          = "k8s-namespace"
)

type ConsulService struct {
	Namespace string
	Name      string
}

type BackendReference struct {
	HTTPRef    *gwv1alpha2.HTTPBackendRef
	BackendRef *gwv1alpha2.BackendRef
}

// TODO: this will require a little extra work to return
// gwv1alpha2.BackendObjectReference for TCPRoute BackendRef
// and gwv1beta1.BackendObjectReference for HTTPRoute HTTPBackendRef
func (b *BackendReference) Set(reference interface{}) {
	switch ref := reference.(type) {
	case *gwv1alpha2.HTTPBackendRef:
		b.HTTPRef = ref
	case *gwv1alpha2.BackendRef:
		b.BackendRef = ref
	}
}

type ResolvedReference struct {
	Type      ResolvedReferenceType
	Reference *BackendReference
	Consul    *ConsulService
}

func NewConsulServiceReference(consul *ConsulService) *ResolvedReference {
	return &ResolvedReference{
		Type:      ConsulServiceReference,
		Reference: &BackendReference{},
		Consul:    consul,
	}
}

type BackendResolver interface {
	Resolve(ctx context.Context, namespace string, ref gwv1alpha2.BackendObjectReference) (*ResolvedReference, error)
}

type backendResolver struct {
	client gatewayclient.Client
	consul consul.Client
	logger hclog.Logger
	mapper common.ConsulNamespaceMapper
}

func NewBackendResolver(logger hclog.Logger, mapper common.ConsulNamespaceMapper, client gatewayclient.Client, consul consul.Client) *backendResolver {
	return &backendResolver{
		client: client,
		consul: consul,
		mapper: mapper,
		logger: logger,
	}
}

func (r *backendResolver) Resolve(ctx context.Context, namespace string, ref gwv1alpha2.BackendObjectReference) (*ResolvedReference, error) {
	group := corev1.GroupName
	kind := "Service"
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
	case group == apigwv1alpha1.GroupVersion.Group && kind == apigwv1alpha1.MeshServiceKind:
		return r.consulServiceForMeshService(ctx, namespacedName)
	default:
		return nil, NewInvalidKindError(fmt.Sprintf("unsupported reference kind %s", kind))
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
		return nil, NewBackendNotFoundError(fmt.Sprintf("service %s not found", namespacedName))
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
			return NewBackendNotFoundError(fmt.Sprintf("consul service %s not found", namespacedName))
		}
		return nil
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
	if err != nil {
		r.logger.Error("could not resolve consul service", "error", err)
		return nil, err
	}
	return resolved, nil
}

func validateAgentConsulReference(services map[string]*api.AgentService) (*ResolvedReference, error) {
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
	return NewConsulServiceReference(&ConsulService{
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

	var consulNamespaces []*api.Namespace
	// the default namespace
	namespaces := []string{""}
	consulNamespaces, _, err = r.consul.Namespaces().List(nil)
	if err != nil {
		if !strings.Contains(err.Error(), "Unexpected response code: 404") {
			r.logger.Trace("error retrieving namespaces", "error", err)
			return nil, err
		}
		// we're dealing with an OSS version of Consul, skip namespaces other than the
		// default namespace
	} else {
		for _, namespace := range consulNamespaces {
			namespaces = append(namespaces, namespace.Name)
		}
	}

	filter := fmt.Sprintf(`Meta[%q] == %q and Meta[%q] == %q and Kind != "connect-proxy"`, MetaKeyKubeServiceName, service.Name, MetaKeyKubeNS, service.Namespace)
	for _, node := range nodes {
		for _, namespace := range namespaces {
			nodeWithServices, _, err := r.consul.Catalog().Node(node.Node, &api.QueryOptions{
				Filter:    filter,
				Namespace: namespace,
			})
			if err != nil {
				r.logger.Trace("error retrieving node services", "error", err, "node", node.Node)
				return nil, err
			}
			if len(nodeWithServices.Services) == 0 {
				continue
			}
			resolved, err := validateAgentConsulReference(nodeWithServices.Services)
			if err != nil {
				r.logger.Trace("error validating node services", "error", err, "node", node.Node)
				return nil, err
			}
			if resolved != nil {
				return resolved, nil
			}
		}
	}
	return nil, nil
}

func (r *backendResolver) consulServiceForMeshService(ctx context.Context, namespacedName types.NamespacedName) (*ResolvedReference, error) {
	var err error
	var resolved *ResolvedReference

	service, err := r.client.GetMeshService(ctx, namespacedName)
	if err != nil {
		r.logger.Trace("error retrieving mesh service", "error", err, "name", namespacedName.Name, "namespace", namespacedName.Namespace)
		return nil, err
	}
	if service == nil {
		return nil, NewBackendNotFoundError(fmt.Sprintf("kubernetes mesh service object %s not found", namespacedName))
	}

	// we do an inner retry since consul may take some time to sync
	err = backoff.Retry(func() error {
		r.logger.Trace("attempting to resolve global catalog service")

		if pointer.StringDeref(service.Spec.Peer, "") != "" {
			resolved, err = r.findPeerService(ctx, service)
			if err != nil {
				r.logger.Trace("error resolving imported service reference")
				return err
			} else if resolved == nil {
				return NewConsulResolutionError(
					fmt.Sprintf("imported consul service %s from peer %s not found", namespacedName, *service.Spec.Peer))
			}
		} else {
			resolved, err = r.findCatalogService(service)
			if err != nil {
				r.logger.Trace("error resolving global catalog reference", "error", err)
				return err
			} else if resolved == nil {
				return NewConsulResolutionError(fmt.Sprintf("consul service %s not found", namespacedName))
			}
		}

		return nil
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 30), ctx))
	if err != nil {
		r.logger.Error("could not resolve consul service", "error", err)
		return nil, err
	}
	return resolved, nil
}

func (r *backendResolver) findPeerService(ctx context.Context, service *apigwv1alpha1.MeshService) (*ResolvedReference, error) {
	if pointer.StringDeref(service.Spec.Peer, "") == "" {
		return nil, NewConsulResolutionError("peer name expected but not provided")
	}

	consulNamespace := r.mapper(service.Namespace)
	consulName := service.Spec.Name
	consulPeer := *service.Spec.Peer

	peer, _, err := r.consul.Peerings().Read(ctx, consulPeer, &api.QueryOptions{Namespace: consulNamespace})
	if err != nil {
		r.logger.Trace("error resolving imported consul service reference", "error", err)
		return nil, err
	}

	if peer == nil {
		return nil, NewConsulResolutionError(fmt.Sprintf("no peer %q found", consulPeer))
	}

	for _, importedService := range peer.StreamStatus.ImportedServices {
		if importedService == consulName {
			return NewConsulServiceReference(&ConsulService{
				Namespace: consulNamespace,
				Name:      consulName,
			}), nil
		}
	}

	return nil, NewConsulResolutionError(fmt.Sprintf("no service %s found from peer %s", consulName, consulPeer))
}

func (r *backendResolver) findCatalogService(service *apigwv1alpha1.MeshService) (*ResolvedReference, error) {
	consulNamespace := r.mapper(service.Namespace)
	consulName := service.Spec.Name
	services, _, err := r.consul.Catalog().Service(consulName, "", &api.QueryOptions{
		Namespace: consulNamespace,
	})
	if err != nil {
		r.logger.Trace("error resolving consul service reference", "error", err)
		return nil, err
	}
	if len(services) == 0 {
		return nil, NewBackendNotFoundError(fmt.Sprintf("consul service (%s, %s) not found", consulNamespace, consulName))
	}
	resolved, err := validateCatalogConsulReference(services)
	if err != nil {
		r.logger.Trace("error validating consul services", "error", err)
		return nil, err
	}
	return resolved, nil
}

func validateCatalogConsulReference(services []*api.CatalogService) (*ResolvedReference, error) {
	serviceName := ""
	serviceNamespace := ""
	for _, service := range services {
		if serviceName == "" {
			serviceName = service.ServiceName
		}
		if serviceNamespace == "" {
			serviceNamespace = service.Namespace
		}
		if service.ServiceName != serviceName || service.Namespace != serviceNamespace {
			return nil,
				NewConsulResolutionError(fmt.Sprintf(
					"must have a single service map to a kubernetes service, found - (%q, %q) and (%q, %q)",
					serviceNamespace, serviceName, service.Namespace, service.ServiceName,
				))
		}
	}
	return NewConsulServiceReference(&ConsulService{
		Name:      serviceName,
		Namespace: serviceNamespace,
	}), nil
}
