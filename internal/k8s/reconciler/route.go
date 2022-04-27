package reconciler

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/converters"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
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
}

var _ store.Route = &K8sRoute{}

type storedRoute struct {
	GVK        schema.GroupVersionKind
	Route      []byte
	RouteState *state.RouteState
}

func NewRoute(route Route, state *state.RouteState) *K8sRoute {
	return &K8sRoute{
		Route:      route,
		RouteState: state,
	}
}

func (r *K8sRoute) ID() string {
	switch r.Route.(type) {
	case *gw.HTTPRoute:
		return HTTPRouteID(utils.NamespacedName(r.Route))
	case *gw.TCPRoute:
		return TCPRouteID(utils.NamespacedName(r.Route))
	}
	return ""
}

func (r *K8sRoute) matchesHostname(hostname *gw.Hostname) bool {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return routeMatchesListenerHostname(hostname, route.Spec.Hostnames)
	default:
		return true
	}
}

func routeMatchesListenerHostname(listenerHostname *gw.Hostname, hostnames []gw.Hostname) bool {
	if listenerHostname == nil || len(hostnames) == 0 {
		return true
	}

	for _, name := range hostnames {
		if hostnamesMatch(name, *listenerHostname) {
			return true
		}
	}
	return false
}

func hostnamesMatch(a, b gw.Hostname) bool {
	if a == "" || a == "*" || b == "" || b == "*" {
		// any wildcard always matches
		return true
	}

	if strings.HasPrefix(string(a), "*.") || strings.HasPrefix(string(b), "*.") {
		aLabels, bLabels := strings.Split(string(a), "."), strings.Split(string(b), ".")
		if len(aLabels) != len(bLabels) {
			return false
		}

		for i := 1; i < len(aLabels); i++ {
			if !strings.EqualFold(aLabels[i], bLabels[i]) {
				return false
			}
		}
		return true
	}

	return a == b
}

func (r *K8sRoute) commonRouteSpec() gw.CommonRouteSpec {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.TCPRoute:
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
	}
	return gw.RouteStatus{}
}

func (r *K8sRoute) setStatus(updated gw.RouteStatus) {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = updated
	case *gw.TCPRoute:
		route.Status.RouteStatus = updated
	}
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

func (r *K8sRoute) parents() []gw.ParentRef {
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

func HTTPRouteID(namespacedName types.NamespacedName) string {
	return "http-" + namespacedName.String()
}

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}
