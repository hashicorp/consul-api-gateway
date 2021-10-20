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
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// all kubernetes routes implement the following two interfaces
type Route interface {
	client.Object
	schema.ObjectKind
}

type K8sRoute struct {
	Route

	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
	resolver       service.BackendResolver

	references       service.RouteRuleReferenceMap
	resolutionErrors *service.ResolutionErrors

	parentStatuses map[string]*RouteStatus
}

var _ store.StatusTrackingRoute = &K8sRoute{}

type K8sRouteConfig struct {
	ControllerName string
	Logger         hclog.Logger
	Client         gatewayclient.Client
	Resolver       service.BackendResolver
}

func NewK8sRoute(route Route, config K8sRouteConfig) *K8sRoute {
	return &K8sRoute{
		Route:            route,
		controllerName:   config.ControllerName,
		logger:           config.Logger.Named("route").With("name", route.GetName()),
		client:           config.Client,
		resolver:         config.Resolver,
		references:       service.RouteRuleReferenceMap{},
		resolutionErrors: service.NewResolutionErrors(),
		parentStatuses:   make(map[string]*RouteStatus),
	}
}

func (r *K8sRoute) parentKeyForGateway(parent types.NamespacedName) (string, bool) {
	for _, p := range r.Parents() {
		gatewayName, isGateway := utils.ReferencesGateway(r.GetNamespace(), p)
		if isGateway && gatewayName == parent {
			return asJSON(p), true
		}
	}
	return "", false
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
		route.Status.RouteStatus = updated
	case *gw.TCPRoute:
		route.Status.RouteStatus = updated
	case *gw.UDPRoute:
		route.Status.RouteStatus = updated
	case *gw.TLSRoute:
		route.Status.RouteStatus = updated
	}
}

func (r *K8sRoute) ParentStatuses() []gw.RouteParentStatus {
	statuses := []gw.RouteParentStatus{}
	for ref, status := range r.parentStatuses {
		statuses = append(statuses, gw.RouteParentStatus{
			ParentRef:      parseParent(ref),
			ControllerName: gw.GatewayController(r.controllerName),
			Conditions:     status.Conditions(r.GetGeneration()),
		})
	}
	return statuses
}

func (r *K8sRoute) FilterParentStatuses() []gw.RouteParentStatus {
	filtered := []gw.RouteParentStatus{}
	for _, status := range r.routeStatus().Parents {
		if status.ControllerName != gw.GatewayController(r.controllerName) {
			filtered = append(filtered, status)
			continue
		}
	}
	return filtered
}

func (r *K8sRoute) MergedStatus() gw.RouteStatus {
	return gw.RouteStatus{
		Parents: sortParents(append(r.FilterParentStatuses(), r.ParentStatuses()...)),
	}
}

func (r *K8sRoute) NeedsStatusUpdate() bool {
	currentStatus := gw.RouteStatus{Parents: sortParents(r.routeStatus().Parents)}
	updatedStatus := r.MergedStatus()
	return !routeStatusEqual(currentStatus, updatedStatus)
}

func (r *K8sRoute) OnBindFailed(err error, gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.gateway))
		if found {
			status, statusFound := r.parentStatuses[id]
			if !statusFound {
				status = &RouteStatus{}
			}
			var bindError BindError
			if errors.As(err, &bindError) {
				switch bindError.Kind() {
				case BindErrorTypeHostnameMismatch:
					status.Accepted.ListenerHostnameMismatch = err
				case BindErrorTypeListenerNamespacePolicy:
					status.Accepted.ListenerNamespacePolicy = err
				case BindErrorTypeRouteKind:
					status.Accepted.InvalidRouteKind = err
				}
				return
			}
			status.Accepted.BindError = err
			r.parentStatuses[id] = status
		}
	}
}

func (r *K8sRoute) OnBound(gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.gateway))
		if found {
			// clear out any existing errors on the accepted status
			if status, statusFound := r.parentStatuses[id]; statusFound {
				status.Accepted = RouteAcceptedStatus{}
			} else {
				r.parentStatuses[id] = &RouteStatus{}
			}
		}
	}
}

func (r *K8sRoute) OnGatewayRemoved(gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.gateway))
		if found {
			delete(r.parentStatuses, id)
		}
	}
}

func (r *K8sRoute) SyncStatus(ctx context.Context) error {
	if r.NeedsStatusUpdate() {
		status := r.routeStatus()
		r.SetStatus(r.MergedStatus())

		if r.logger.IsTrace() {
			status, err := json.MarshalIndent(r.routeStatus(), "", "  ")
			if err == nil {
				r.logger.Trace("syncing route status", "status", string(status))
			}
		}
		if err := r.client.UpdateStatus(ctx, r.Route); err != nil {
			// reset the status so we sync again on a retry
			r.SetStatus(status)
			return fmt.Errorf("error updating route status: %w", err)
		}
	}

	return nil
}

func (r *K8sRoute) Compare(other store.Route) store.CompareResult {
	if other == nil {
		return store.CompareResultInvalid
	}
	if r == nil {
		return store.CompareResultNotEqual
	}

	if otherRoute, ok := other.(*K8sRoute); ok {
		if utils.ResourceVersionGreater(r.GetResourceVersion(), otherRoute.GetResourceVersion()) {
			return store.CompareResultNewer
		}

		if r.isEqual(otherRoute) {
			return store.CompareResultEqual
		}
		return store.CompareResultNotEqual
	}
	return store.CompareResultInvalid
}

func (r *K8sRoute) isEqual(k8sRoute *K8sRoute) bool {
	if !reflect.DeepEqual(r.references, k8sRoute.references) || !reflect.DeepEqual(r.resolutionErrors, k8sRoute.resolutionErrors) {
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

func (r *K8sRoute) Resolve(listener store.Listener) *core.ResolvedRoute {
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
			"external-source":                            "consul-api-gateway",
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
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		for _, rule := range route.Spec.Rules {
			routeRule := service.NewRouteRule(&rule)
			for _, ref := range rule.BackendRefs {
				reference, err := r.resolver.Resolve(ctx, ref.BackendObjectReference)
				if err != nil {
					var resolutionError service.ResolutionError
					if !errors.As(err, &resolutionError) {
						return err
					}
					r.resolutionErrors.Add(resolutionError)
					continue
				}
				reference.Reference.Set(&ref)
				r.references.Add(routeRule, *reference)
			}
		}
	}

	return nil
}

func (r *K8sRoute) IsValid() bool {
	return r.resolutionErrors.Empty()
}
