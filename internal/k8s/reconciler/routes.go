package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/state"
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

	controllerName  string
	logger          hclog.Logger
	client          gatewayclient.Client
	consul          *api.Client
	references      routeRuleReferenceMap
	needsStatusSync bool
	isResolved      bool
}

var _ state.Route = &K8sRoute{}

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

func (r *K8sRoute) ID() string {
	namespacedName := utils.NamespacedName(r.Route).String()

	switch r.Route.(type) {
	case *gw.HTTPRoute:
		return "http-" + namespacedName
	case *gw.TCPRoute:
		return "tcp-" + namespacedName
	case *gw.UDPRoute:
		return "udp-" + namespacedName
	case *gw.TLSRoute:
		return "tls-" + namespacedName
	}
	return ""
}

func (r *K8sRoute) Logger() hclog.Logger {
	return r.logger
}

func (r *K8sRoute) MatchesHostname(hostname *gw.Hostname) bool {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return routeMatchesListenerHostname(hostname, route.Spec.Hostnames)
	default:
		return true
	}
}

func (r *K8sRoute) CommonRouteSpec() gw.CommonRouteSpec {
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

func (r *K8sRoute) SetStatus(updated gw.RouteStatus) {
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
		r.needsStatusSync = true
		return updated
	}

	if !reflect.DeepEqual(current, updated) {
		r.needsStatusSync = true
		return updated
	}
	return current
}

func (r *K8sRoute) OnBindFailed(err error, gateway state.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		r.SetStatus(setAdmittedStatus(r.routeStatus(), gw.RouteParentStatus{
			ParentRef:  r.getRef(k8sGateway.gateway),
			Controller: gw.GatewayController(r.controllerName),
			Conditions: []metav1.Condition{{
				Status:             metav1.ConditionFalse,
				Reason:             ConditionReasonUnableToBind,
				Message:            err.Error(),
				Type:               string(gw.ConditionRouteAdmitted),
				ObservedGeneration: r.GetGeneration(),
				LastTransitionTime: metav1.Now(),
			}},
		}))
	}
}

func (r *K8sRoute) OnBound(gateway state.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		r.SetStatus(setAdmittedStatus(r.routeStatus(), gw.RouteParentStatus{
			ParentRef:  r.getRef(k8sGateway.gateway),
			Controller: gw.GatewayController(r.controllerName),
			Conditions: []metav1.Condition{{
				Status:             metav1.ConditionTrue,
				Reason:             ConditionReasonRouteAdmitted,
				Message:            "Route allowed",
				Type:               string(gw.ConditionRouteAdmitted),
				ObservedGeneration: r.GetGeneration(),
				LastTransitionTime: metav1.Now(),
			}},
		}))
	}
}

func (r *K8sRoute) OnGatewayRemoved(gateway state.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		r.SetStatus(clearParentStatus(
			r.controllerName,
			r.GetName(),
			r.routeStatus(),
			utils.NamespacedName(k8sGateway.gateway)))
	}
}

func (r *K8sRoute) getRef(gateway *gw.Gateway) gw.ParentRef {
	namespacedName := utils.NamespacedName(gateway)
	for _, ref := range r.CommonRouteSpec().ParentRefs {
		namedGateway, isGatewayRef := referencesGateway(r.GetNamespace(), ref)
		if isGatewayRef && namedGateway == namespacedName {
			return ref
		}
	}
	return gw.ParentRef{}
}

func (r *K8sRoute) SyncStatus(ctx context.Context) error {
	if r.needsStatusSync {
		if r.logger.IsTrace() {
			status, err := json.MarshalIndent(r.routeStatus(), "", "  ")
			if err == nil {
				r.logger.Trace("syncing route status", "status", string(status))
			}
		}
		if err := r.client.UpdateStatus(ctx, r.Route); err != nil {
			return fmt.Errorf("error updating route status: %w", err)
		}

		r.needsStatusSync = false
	}

	return nil
}

func (r *K8sRoute) IsMoreRecent(other state.Route) bool {
	k8sRoute, ok := other.(*K8sRoute)
	if !ok {
		return true
	}

	return r.GetGeneration() > k8sRoute.GetGeneration()
}

func (r *K8sRoute) Equals(other state.Route) bool {
	k8sRoute, ok := other.(*K8sRoute)
	if !ok {
		return false
	}

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		if otherRoute, ok := k8sRoute.Route.(*gw.HTTPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.TCPRoute:
		if otherRoute, ok := k8sRoute.Route.(*gw.TCPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.UDPRoute:
		if otherRoute, ok := k8sRoute.Route.(*gw.UDPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.TLSRoute:
		if otherRoute, ok := k8sRoute.Route.(*gw.TLSRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	}
	return false
}

func (r *K8sRoute) DiscoveryChain(listener state.Listener) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	k8sListener, ok := listener.(*K8sListener)
	if !ok {
		return nil, nil, nil, nil
	}

	prefix := fmt.Sprintf("consul-api-gateway_%s_", k8sListener.gateway.Name)
	hostname := k8sListener.Config().Hostname
	meta := map[string]string{
		"managed_by":                                 "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":        k8sListener.gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace":   k8sListener.gateway.Namespace,
		"consul-api-gateway/k8s/HTTPRoute.Name":      r.GetName(),
		"consul-api-gateway/k8s/HTTPRoute.Namespace": r.GetNamespace(),
	}

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

func (r *K8sRoute) ResolveServices(ctx context.Context) error {
	var result error

	if r.isResolved {
		return nil
	}

	resolved := routeRuleReferenceMap{}
	var parents []gw.ParentRef
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		for _, rule := range route.Spec.Rules {
			parents = route.Spec.ParentRefs
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

	r.SetStatus(setResolvedRefsStatus(r.routeStatus(), statuses...))

	if result == nil {
		r.references = resolved
		r.isResolved = true
	}
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
