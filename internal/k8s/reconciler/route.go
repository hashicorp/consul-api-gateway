package reconciler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/converters"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
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
	*state.RouteState

	// these get populated by our factory
	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
}

var _ store.StatusTrackingRoute = &K8sRoute{}

func (r *K8sRoute) ID() string {
	switch r.Route.(type) {
	case *gw.HTTPRoute:
		return HTTPRouteID(utils.NamespacedName(r.Route))
	case *gw.TCPRoute:
		return TCPRouteID(utils.NamespacedName(r.Route))
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

func (r *K8sRoute) SyncStatus(ctx context.Context) error {
	if status, ok := r.RouteState.ParentStatuses.NeedsUpdate(r.routeStatus(), r.controllerName, r.GetGeneration()); ok {
		r.SetStatus(status)

		if r.logger.IsTrace() {
			status, err := json.MarshalIndent(status, "", "  ")
			if err == nil {
				r.logger.Trace("syncing route status", "status", string(status))
			}
		}
		if err := r.client.UpdateStatus(ctx, r.Route); err != nil {
			return fmt.Errorf("error updating route status: %w", err)
		}
	}

	return nil
}

func (r *K8sRoute) resolve(namespace string, gateway *gw.Gateway, listener gw.Listener) core.ResolvedRoute {
	hostname := listenerHostname(listener)
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return converters.NewHTTPRouteConverter(converters.HTTPRouteConverterConfig{
			Namespace: namespace,
			Hostname:  hostname,
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
	case *gw.TCPRoute:
		return converters.NewTCPRouteConverter(converters.TCPRouteConverterConfig{
			Namespace: namespace,
			Hostname:  hostname,
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
	}
	return nil
}

func (r *K8sRoute) Parents() []gw.ParentRef {
	// filter for this controller
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Spec.ParentRefs
	case *gw.TCPRoute:
		return route.Spec.ParentRefs
	}
	return nil
}

func HTTPRouteID(namespacedName types.NamespacedName) string {
	return "http-" + namespacedName.String()
}

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}
