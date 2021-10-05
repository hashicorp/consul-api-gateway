package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/state"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

var _ state.StatusTrackingRoute = &K8sRoute{}
var _ state.InitializableRoute = &K8sRoute{}

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
	switch r.Route.(type) {
	case *gw.HTTPRoute:
		return HTTPRouteID(utils.NamespacedName(r.Route))
	case *gw.TCPRoute:
		return TCPRouteID(utils.NamespacedName(r.Route))
	case *gw.UDPRoute:
		return UDPRouteID(utils.NamespacedName(r.Route))
	case *gw.TLSRoute:
		return TLSRouteID(utils.NamespacedName(r.Route))
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

func (r *K8sRoute) Compare(other state.Route) state.CompareResult {
	if other == nil {
		return state.CompareResultInvalid
	}
	if r == nil {
		return state.CompareResultNotEqual
	}

	if otherRoute, ok := other.(*K8sRoute); ok {
		if r.GetGeneration() > otherRoute.GetGeneration() {
			return state.CompareResultNewer
		}
		if r.equals(otherRoute) {
			return state.CompareResultEqual
		}
		return state.CompareResultNotEqual
	}
	return state.CompareResultInvalid
}

func (r *K8sRoute) equals(k8sRoute *K8sRoute) bool {
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

func (r *K8sRoute) Resolve(listener state.Listener) *state.ResolvedRoute {
	k8sListener, ok := listener.(*K8sListener)
	if !ok {
		return nil
	}

	prefix := fmt.Sprintf("consul-api-gateway_%s_", k8sListener.gateway.Name)
	hostname := k8sListener.Config().Hostname
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return convertHTTPRoute(hostname, prefix, map[string]string{
			"managed_by":                                 "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":        k8sListener.gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace":   k8sListener.gateway.Namespace,
			"consul-api-gateway/k8s/HTTPRoute.Name":      r.GetName(),
			"consul-api-gateway/k8s/HTTPRoute.Namespace": r.GetNamespace(),
		}, route, r)
	default:
		// TODO: add other route types
		return nil
	}
}

func (r *K8sRoute) Init(ctx context.Context) error {
	var result error

	if r.isResolved {
		return nil
	}

	resolver := newBackendResolver(r.GetNamespace(), r.client, r.consul)
	resolved := routeRuleReferenceMap{}
	var parents []gw.ParentRef
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		for _, rule := range route.Spec.Rules {
			parents = route.Spec.ParentRefs
			routeRule := routeRule{httpRule: &rule}
			for _, ref := range rule.BackendRefs {
				resolvedRef, err := resolver.resolveBackendReference(ctx, ref.BackendObjectReference)
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
		return nil
	}
	return result
}
