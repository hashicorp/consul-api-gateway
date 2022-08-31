package reconciler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/converter"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/validator"
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
	validator      *validator.RouteValidator
}

var _ store.StatusTrackingRoute = &K8sRoute{}

type K8sRouteConfig struct {
	ControllerName string
	Logger         hclog.Logger
	Client         gatewayclient.Client
	Resolver       service.BackendResolver
}

func newK8sRoute(route Route, config K8sRouteConfig) *K8sRoute {
	return &K8sRoute{
		Route:          route,
		RouteState:     state.NewRouteState(),
		controllerName: config.ControllerName,
		logger:         config.Logger.Named("route").With("name", route.GetName()),
		client:         config.Client,
		validator:      validator.NewRouteValidator(config.Resolver, config.Client),
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

func (r *K8sRoute) matchesHostname(hostname *gwv1beta1.Hostname) bool {
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

func (r *K8sRoute) Resolve(listener store.Listener) core.ResolvedRoute {
	k8sListener, ok := listener.(*K8sListener)
	if !ok {
		return nil
	}

	return r.resolve(k8sListener.consulNamespace, k8sListener.gateway, k8sListener.listener)
}

func (r *K8sRoute) resolve(namespace string, gateway *gwv1beta1.Gateway, listener gwv1beta1.Listener) core.ResolvedRoute {
	hostname := listenerHostname(listener)

	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return converter.NewHTTPRouteConverter(converter.HTTPRouteConverterConfig{
			Namespace: namespace,
			Hostname:  hostname,
			Prefix:    fmt.Sprintf("consul-api-gateway_%s_", gateway.Name),
			Meta: map[string]string{
				"external-source":                            "consul-api-gateway",
				"consul-api-gateway/k8s/Gateway.Name":        gateway.Name,
				"consul-api-gateway/k8s/Gateway.Namespace":   gateway.Namespace,
				"consul-api-gateway/k8s/HTTPRoute.Name":      r.GetName(),
				"consul-api-gateway/k8s/HTTPRoute.Namespace": r.GetNamespace(),
			},
			Route: route,
			State: r.RouteState,
		}).Convert()
	case *gwv1alpha2.TCPRoute:
		return converter.NewTCPRouteConverter(converter.TCPRouteConverterConfig{
			Namespace: namespace,
			Hostname:  hostname,
			Prefix:    fmt.Sprintf("consul-api-gateway_%s_", gateway.Name),
			Meta: map[string]string{
				"external-source":                           "consul-api-gateway",
				"consul-api-gateway/k8s/Gateway.Name":       gateway.Name,
				"consul-api-gateway/k8s/Gateway.Namespace":  gateway.Namespace,
				"consul-api-gateway/k8s/TCPRoute.Name":      r.GetName(),
				"consul-api-gateway/k8s/TCPRoute.Namespace": r.GetNamespace(),
			},
			Route: route,
			State: r.RouteState,
		}).Convert()
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
	return r.validator.Validate(ctx, r.RouteState, r.Route)
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

func HTTPRouteID(namespacedName types.NamespacedName) string {
	return "http-" + namespacedName.String()
}

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}
