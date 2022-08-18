package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

// all kubernetes routes implement the following two interfaces
type Route interface {
	client.Object
	schema.ObjectKind
}

type K8sRoute struct {
	Route
	RouteState *state.RouteState

	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
	resolver       service.BackendResolver
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
		Route:          route,
		RouteState:     state.NewRouteState(),
		controllerName: config.ControllerName,
		logger:         config.Logger.Named("route").With("name", route.GetName()),
		client:         config.Client,
		resolver:       config.Resolver,
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
	case *gwv1alpha2.HTTPRoute:
		return HTTPRouteID(utils.NamespacedName(r.Route))
	case *gwv1alpha2.TCPRoute:
		return TCPRouteID(utils.NamespacedName(r.Route))
	}
	return ""
}

func (r *K8sRoute) MatchesHostname(hostname *gwv1beta1.Hostname) bool {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return routeMatchesListenerHostname(hostname, route.Spec.Hostnames)
	default:
		return true
	}
}

func (r *K8sRoute) CommonRouteSpec() gwv1alpha2.CommonRouteSpec {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return route.Spec.CommonRouteSpec
	case *gwv1alpha2.TCPRoute:
		return route.Spec.CommonRouteSpec
	}
	return gwv1alpha2.CommonRouteSpec{}
}

func (r *K8sRoute) routeStatus() gwv1alpha2.RouteStatus {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return route.Status.RouteStatus
	case *gwv1alpha2.TCPRoute:
		return route.Status.RouteStatus
	}
	return gwv1alpha2.RouteStatus{}
}

func (r *K8sRoute) SetStatus(updated gwv1alpha2.RouteStatus) {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		route.Status.RouteStatus = updated
	case *gwv1alpha2.TCPRoute:
		route.Status.RouteStatus = updated
	}
}

func (r *K8sRoute) SyncStatus(ctx context.Context) error {
	if status, ok := r.RouteState.ParentStatuses.NeedsUpdate(r.routeStatus(), r.controllerName, r.GetGeneration()); ok {
		r.SetStatus(status)

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

func (r *K8sRoute) Resolve(listener store.Listener) *core.ResolvedRoute {
	k8sListener, ok := listener.(*K8sListener)
	if !ok {
		return nil
	}

	prefix := fmt.Sprintf("consul-api-gateway_%s_", k8sListener.gateway.Name)
	namespace := k8sListener.consulNamespace
	hostname := k8sListener.Config().Hostname
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return convertHTTPRoute(namespace, hostname, prefix, map[string]string{
			"external-source":                            "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":        k8sListener.gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace":   k8sListener.gateway.Namespace,
			"consul-api-gateway/k8s/HTTPRoute.Name":      r.GetName(),
			"consul-api-gateway/k8s/HTTPRoute.Namespace": r.GetNamespace(),
		}, route, r)
	case *gwv1alpha2.TCPRoute:
		return convertTCPRoute(namespace, prefix, map[string]string{
			"external-source":                           "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":       k8sListener.gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace":  k8sListener.gateway.Namespace,
			"consul-api-gateway/k8s/TCPRoute.Name":      r.GetName(),
			"consul-api-gateway/k8s/TCPRoute.Namespace": r.GetNamespace(),
		}, route, r)
	default:
		// TODO: add other route types
		return nil
	}
}

func (r *K8sRoute) Parents() []gwv1alpha2.ParentReference {
	// filter for this controller
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return route.Spec.ParentRefs
	case *gwv1alpha2.TCPRoute:
		return route.Spec.ParentRefs
	}
	return nil
}

func (r *K8sRoute) Validate(ctx context.Context) error {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		for _, httpRule := range route.Spec.Rules {
			rule := httpRule
			routeRule := service.NewRouteRule(&rule)

			for _, backendRef := range rule.BackendRefs {
				ref := backendRef

				allowed, err := routeAllowedForBackendRef(ctx, r.Route, ref.BackendRef, r.client)
				if err != nil {
					return err
				} else if !allowed {
					nsName := getNamespacedName(ref.Name, ref.Namespace, route.Namespace)
					msg := fmt.Sprintf("Cross-namespace routing not allowed without matching ReferenceGrant for Service %q", nsName)
					r.logger.Warn("Cross-namespace routing not allowed without matching ReferenceGrant", "refName", nsName.Name, "refNamespace", nsName.Namespace)
					r.RouteState.ResolutionErrors.Add(service.NewRefNotPermittedError(msg))
					continue
				}

				reference, err := r.resolver.Resolve(ctx, r.GetNamespace(), ref.BackendObjectReference)
				if err != nil {
					var resolutionError service.ResolutionError
					if !errors.As(err, &resolutionError) {
						return err
					}
					r.RouteState.ResolutionErrors.Add(resolutionError)
					continue
				}
				reference.Reference.Set(&ref)
				r.RouteState.References.Add(routeRule, *reference)
			}
		}
	case *gwv1alpha2.TCPRoute:
		if len(route.Spec.Rules) != 1 {
			err := service.NewResolutionError("a single tcp rule is required")
			r.RouteState.ResolutionErrors.Add(err)
			return nil
		}

		rule := route.Spec.Rules[0]

		if len(rule.BackendRefs) != 1 {
			err := service.NewResolutionError("a single backendRef per tcp rule is required")
			r.RouteState.ResolutionErrors.Add(err)
			return nil
		}

		routeRule := service.NewRouteRule(rule)

		ref := rule.BackendRefs[0]

		allowed, err := routeAllowedForBackendRef(ctx, r.Route, ref, r.client)
		if err != nil {
			return err
		} else if !allowed {
			nsName := getNamespacedName(ref.Name, ref.Namespace, route.Namespace)
			msg := fmt.Sprintf("Cross-namespace routing not allowed without matching ReferenceGrant for Service %q", nsName)
			r.logger.Warn("Cross-namespace routing not allowed without matching ReferenceGrant", "refName", nsName.Name, "refNamespace", nsName.Namespace)
			r.RouteState.ResolutionErrors.Add(service.NewRefNotPermittedError(msg))
			return nil
		}

		reference, err := r.resolver.Resolve(ctx, r.GetNamespace(), ref.BackendObjectReference)
		if err != nil {
			var resolutionError service.ResolutionError
			if !errors.As(err, &resolutionError) {
				return err
			}
			r.RouteState.ResolutionErrors.Add(resolutionError)
			return nil
		}

		reference.Reference.Set(&ref)
		r.RouteState.References.Add(routeRule, *reference)
	}

	return nil
}

func (r *K8sRoute) OnBindFailed(err error, gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.Gateway))
		if found {
			r.RouteState.ParentStatuses.BindFailed(r.RouteState.ResolutionErrors, err, id)
		}
	}
}

func (r *K8sRoute) OnBound(gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.Gateway))
		if found {
			r.RouteState.ParentStatuses.Bound(id)
		}
	}
}

func (r *K8sRoute) OnGatewayRemoved(gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.Gateway))
		if found {
			r.RouteState.ParentStatuses.Remove(id)
		}
	}
}

func (r *K8sRoute) IsValid() bool {
	return r.RouteState.ResolutionErrors.Empty()
}
