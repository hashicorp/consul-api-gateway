package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type resolvedReferenceType int

var (
	ErrEmptyPort            = errors.New("port cannot be empty with kubernetes service")
	ErrNotResolved          = errors.New("backend reference not found")
	ErrConsulNotResolved    = errors.New("consul service not found")
	ErrUnsupportedReference = errors.New("unsupported reference type")
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel     = "kubernetes.io/metadata.name"
	MetaKeyKubeServiceName = "k8s-service-name"
	MetaKeyKubeNS          = "k8s-namespace"
)

// all kubernetes routes implement the following two interfaces
type Route interface {
	client.Object
	schema.ObjectKind
}

type RouteRule struct {
	httpRule *gw.HTTPRouteRule
}

type K8sRoute struct {
	Route

	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
	consul         *api.Client

	needsStatusSync bool
	isResolved      bool
	references      map[RouteRule][]resolvedReference

	// this mutex protects any of the above private fields
	mutex sync.RWMutex
}

type K8sRouteConfig struct {
	ControllerName string
	Logger         hclog.Logger
	Client         gatewayclient.Client
	Consul         *api.Client
}

func NewK8sRoute(route Route, config K8sRouteConfig) *K8sRoute {
	return &K8sRoute{
		Route:           route,
		controllerName:  config.ControllerName,
		logger:          config.Logger.Named("route").With("name", route.GetName()),
		client:          config.Client,
		consul:          config.Consul,
		needsStatusSync: true,
	}
}

func (r *K8sRoute) MatchesHostname(hostname *gw.Hostname) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return routeMatchesListenerHostname(hostname, route.Spec.Hostnames)
	default:
		return true
	}
}

func (r *K8sRoute) CommonRouteSpec() gw.CommonRouteSpec {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.TCPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.UDPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.TLSRoute:
		return route.Spec.CommonRouteSpec
	}
	return gw.CommonRouteSpec{}
}

func (r *K8sRoute) RouteStatus() gw.RouteStatus {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.routeStatus()
}

func (r *K8sRoute) routeStatus() gw.RouteStatus {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Status.RouteStatus
	case *gw.TCPRoute:
		return route.Status.RouteStatus
	case *gw.UDPRoute:
		return route.Status.RouteStatus
	case *gw.TLSRoute:
		return route.Status.RouteStatus
	}
	return gw.RouteStatus{}
}

func (r *K8sRoute) SetAdmittedStatus(statuses ...gw.RouteParentStatus) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	updated := setAdmittedStatus(r.routeStatus(), statuses...)

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TCPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.UDPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TLSRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	}
}

func (r *K8sRoute) ClearParentStatus(namespacedName types.NamespacedName) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	updated := clearParentStatus(r.controllerName, r.GetName(), r.routeStatus(), namespacedName)

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TCPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.UDPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TLSRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	}
}

// setStatus requires that the statuses always be sorted for equality comparison
func (r *K8sRoute) setStatus(current, updated gw.RouteStatus) gw.RouteStatus {
	if len(current.Parents) != len(updated.Parents) {
		r.logger.Trace("marking route status as dirty")
		r.needsStatusSync = true
		return updated
	}

	if !reflect.DeepEqual(current, updated) {
		r.logger.Trace("marking route status as dirty")
		r.needsStatusSync = true
		return updated
	}
	return current
}

func (r *K8sRoute) setResolvedRefsStatus(statuses ...gw.RouteParentStatus) {
	updated := setResolvedRefsStatus(r.routeStatus(), statuses...)

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TCPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.UDPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TLSRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	}
}

func (r *K8sRoute) ResolveReferences(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.isResolved {
		return nil
	}

	var result error

	resolved := make(map[RouteRule][]resolvedReference)
	var parents []gw.ParentRef
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		parents = route.Spec.ParentRefs
		for _, rule := range route.Spec.Rules {
			routeRule := RouteRule{httpRule: &rule}
			for _, ref := range rule.BackendRefs {
				resolvedRef, err := r.resolveBackendReference(ctx, ref.BackendObjectReference)
				if err != nil {
					result = multierror.Append(result, err)
					continue
				}
				if resolvedRef != nil {
					resolvedRef.ref.Set(&ref)
					resolved[routeRule] = append(resolved[routeRule], *resolvedRef)
				}
			}
		}
	default:
		return nil
	}
	reason := string(gw.ConditionRouteResolvedRefs)
	message := string(gw.ConditionRouteResolvedRefs)
	resolvedStatus := metav1.ConditionTrue
	if result != nil {
		reason = "InvalidRefs"
		message = result.Error()
		resolvedStatus = metav1.ConditionFalse
	}
	conditions := []metav1.Condition{{
		Type:               string(gw.ConditionRouteResolvedRefs),
		Status:             resolvedStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: r.GetGeneration(),
		LastTransitionTime: metav1.Now(),
	}}

	// this seems odd to set this on the parent ref
	statuses := []gw.RouteParentStatus{}
	for _, ref := range parents {
		statuses = append(statuses, gw.RouteParentStatus{
			ParentRef:  ref,
			Controller: gw.GatewayController(r.controllerName),
			Conditions: conditions,
		})
	}

	r.setResolvedRefsStatus(statuses...)

	if result == nil {
		r.isResolved = true
	}
	r.references = resolved
	return result
}

func (r *K8sRoute) resolveBackendReference(ctx context.Context, ref gw.BackendObjectReference) (*resolvedReference, error) {
	group := corev1.GroupName
	kind := "Service"
	namespace := r.GetNamespace()
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

func (r *K8sRoute) consulServiceForK8SService(ctx context.Context, namespacedName types.NamespacedName) (*resolvedReference, error) {
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

func (r *K8sRoute) consulServiceForMeshService(ctx context.Context, namespacedName types.NamespacedName) (*resolvedReference, error) {
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

func (r *K8sRoute) UpdateStatus(ctx context.Context, client gatewayclient.Client) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.needsStatusSync {
		if r.logger.IsTrace() {
			status, err := json.MarshalIndent(r.routeStatus(), "", "  ")
			if err == nil {
				r.logger.Trace("syncing route status", "status", string(status))
			}
		}
		if err := client.UpdateStatus(ctx, r.Route); err != nil {
			return fmt.Errorf("error updating route status: %w", err)
		}
		r.needsStatusSync = false
	}

	return nil
}

func (r *K8sRoute) DiscoveryChain(prefix, hostname string, meta map[string]string) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		router, splits := httpRouteToServiceDiscoChain(r, prefix, meta)
		serviceDefault := httpServiceDefault(router, meta)
		defaults.Add(serviceDefault)
		for _, split := range splits {
			splitters.Add(split)
			if split.Name != serviceDefault.Name {
				defaults.Add(httpServiceDefault(split, meta))
			}
		}

		return &api.IngressService{
			Name:      router.Name,
			Hosts:     utils.HostnamesForHTTPRoute(hostname, route),
			Namespace: "", // TODO
		}, router, splitters, defaults
	}
	return nil, nil, nil, nil
}

func (r *K8sRoute) Equals(other *K8sRoute) bool {
	if r == nil || other == nil {
		return false
	}

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		if otherRoute, ok := other.Route.(*gw.HTTPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.TCPRoute:
		if otherRoute, ok := other.Route.(*gw.TCPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.UDPRoute:
		if otherRoute, ok := other.Route.(*gw.UDPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.TLSRoute:
		if otherRoute, ok := other.Route.(*gw.TLSRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	}
	return false
}
