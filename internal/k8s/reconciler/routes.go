package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel = "kubernetes.io/metadata.name"
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
	references      service.RouteRuleReferenceMap
	needsStatusSync bool
	isResolved      bool

	parentStatuses map[string]RouteStatus
}

var _ core.StatusTrackingRoute = &K8sRoute{}

type K8sRouteConfig struct {
	ControllerName string
	Logger         hclog.Logger
	Client         gatewayclient.Client
	Consul         *api.Client
}

func NewK8sRoute(route Route, config K8sRouteConfig) *K8sRoute {
	r := &K8sRoute{
		Route:           route,
		controllerName:  config.ControllerName,
		logger:          config.Logger.Named("route").With("name", route.GetName()),
		client:          config.Client,
		consul:          config.Consul,
		needsStatusSync: true,
	}

	for _, parent := range r.Parents() {
		r.parentStatuses[stringifyParentRef(parent)] = RouteStatus{}
	}
}

func (r *K8sRoute) parentForGateway(parent types.NamespacedName) (gw.ParentRef, bool) {
	for _, p := range r.Parents() {
		gatewayName, isGateway := referencesGateway(r.GetNamespace(), p)
		if isGateway && gatewayName == parent {
			return p, true
		}
	}
	return gw.ParentRef{}, false
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

func (r *K8sRoute) OnBindFailed(err error, gateway core.Gateway) {
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

func (r *K8sRoute) OnBound(gateway core.Gateway) {
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

func (r *K8sRoute) OnGatewayRemoved(gateway core.Gateway) {
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

func (r *K8sRoute) Compare(other core.Route) core.CompareResult {
	if other == nil {
		return core.CompareResultInvalid
	}
	if r == nil {
		return core.CompareResultNotEqual
	}

	if otherRoute, ok := other.(*K8sRoute); ok {
		if r.GetGeneration() > otherRoute.GetGeneration() {
			return core.CompareResultNewer
		}

		if r.equals(otherRoute) {
			return core.CompareResultEqual
		}
		return core.CompareResultNotEqual
	}
	return core.CompareResultInvalid
}

func (r *K8sRoute) equals(k8sRoute *K8sRoute) bool {
	if !reflect.DeepEqual(r.references, k8sRoute.references) || r.isResolved != k8sRoute.isResolved {
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

func (r *K8sRoute) Resolve(listener core.Listener) *core.ResolvedRoute {
	k8sListener, ok := listener.(*K8sListener)
	if !ok {
		return nil
	}

	prefix := fmt.Sprintf("consul-api-gateway_%s_", k8sListener.gateway.Name)
	namespace := k8sListener.consulNamespace
	hostname := k8sListener.Config().Hostname
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return convertHTTPRoute(namespace, hostname, prefix, map[string]string{
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

func (r *K8sRoute) Parents() []gw.ParentRef {
	// filter for this controller
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Spec.ParentRefs
	case *gw.TCPRoute:
		return route.Spec.ParentRefs
	case *gw.UDPRoute:
		return route.Spec.ParentRefs
	case *gw.TLSRoute:
		return route.Spec.ParentRefs
	}
	return nil
}

func (r *K8sRoute) Validate(ctx context.Context) error {
	resolver := service.NewBackendResolver(r.GetNamespace(), r.client, r.consul)
	resolved := service.RouteRuleReferenceMap{}
	resolutionErrors := service.NewResolutionErrors()

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		for _, rule := range route.Spec.Rules {
			routeRule := service.NewRouteRule(&rule)
			for _, ref := range rule.BackendRefs {
				reference, err := resolver.Resolve(ctx, ref.BackendObjectReference)
				if err != nil {
					var resolutionError service.ResolutionError
					if !errors.As(err, &resolutionError) {
						return err
					}
					resolutionErrors.Add(resolutionError)
					continue
				}
				reference.Reference.Set(&ref)
				resolved.Add(routeRule, *reference)
			}
		}
	default:
		return nil
	}

	if resolutionErrors.Empty() {
		r.isResolved = true
		return nil
	}

	flattenedType, message := resolutionErrors.Flatten()
	for _, parent := range r.Parents() {
		status := r.parentStatuses[stringifyParentRef(parent)]
		switch flattenedType {
		case service.ConsulServiceResolutionErrorType:
			status.ResolvedRefs.ConsulServiceNotFound = message
		case service.K8sServiceResolutionErrorType:
			status.ResolvedRefs.ServiceNotFound = message
		default:
			status.ResolvedRefs.Errors = message
		}
	}

	return nil
}
