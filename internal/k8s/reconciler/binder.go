package reconciler

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel = "kubernetes.io/metadata.name"
)

var _ store.Binder = (*binder)(nil)

// binder wraps encapsulates the logic for binding new Route(s) to a Gateway.
type binder struct {
	Client gatewayclient.Client
}

func NewBinder(client gatewayclient.Client) *binder {
	return &binder{Client: client}
}

// Bind will attempt to bind the provided route to all listeners on the Gateway and
// remove the route from any listeners that the route should no longer be bound to.
// The latter is important for scenarios such as the route's parent changing.
func (b *binder) Bind(ctx context.Context, gateway store.Gateway, route store.Route) bool {
	k8sGateway, k8sRoute := gateway.(*K8sGateway), route.(*K8sRoute)

	var boundListeners []string

	// If the route doesn't reference this Gateway, remove the route
	// from any listeners that it may have previously bound to
	if !b.routeReferencesGateway(k8sRoute, k8sGateway) {
		return b.Unbind(ctx, gateway, route)
	}

	modified := false

	// The route does reference this Gateway, so attempt to bind to each listener
	for _, ref := range k8sRoute.commonRouteSpec().ParentRefs {
		for i, listener := range k8sGateway.Spec.Listeners {
			listenerState := k8sGateway.GatewayState.Listeners[i]
			if b.canBind(ctx, k8sGateway.Namespace, listener, listenerState, ref, k8sRoute) {
				modified = true
				listenerState.Routes[route.ID()] = k8sRoute.resolve(k8sGateway.GatewayState.ConsulNamespace, k8sGateway.Gateway, listener)
				boundListeners = append(boundListeners, string(listener.Name))
			} else {
				// If the route cannot bind to this listener, remove the route in
				// case it was previously bound.
				if _, present := listenerState.Routes[route.ID()]; present {
					modified = true
					delete(listenerState.Routes, route.ID())
				}
			}
		}
	}

	return modified
}

func (b *binder) Unbind(_ context.Context, gateway store.Gateway, route store.Route) bool {
	k8sGateway, k8sRoute := gateway.(*K8sGateway), route.(*K8sRoute)

	removed := false
	for _, listenerState := range k8sGateway.GatewayState.Listeners {
		if _, ok := listenerState.Routes[k8sRoute.ID()]; ok {
			removed = true
			b.removeGatewayStatus(k8sRoute, k8sGateway)
			delete(listenerState.Routes, k8sRoute.ID())
		}
	}

	return removed
}

func (b *binder) removeGatewayStatus(route *K8sRoute, gateway *K8sGateway) {
	thisGateway := utils.NamespacedName(gateway)
	for _, ref := range route.commonRouteSpec().ParentRefs {
		gatewayReferenced, isGatewayTypeRef := utils.ReferencesGateway(route.GetNamespace(), ref)
		if isGatewayTypeRef && gatewayReferenced == thisGateway {
			route.RouteState.Remove(ref)
			return
		}
	}
}

func (b *binder) routeReferencesGateway(route *K8sRoute, gateway *K8sGateway) bool {
	thisGateway := utils.NamespacedName(gateway)
	for _, ref := range route.commonRouteSpec().ParentRefs {
		gatewayReferenced, isGatewayTypeRef := utils.ReferencesGateway(route.GetNamespace(), ref)
		if isGatewayTypeRef && gatewayReferenced == thisGateway {
			return true
		}
	}
	return false
}

func (b *binder) canBind(ctx context.Context, namespace string, listener gwv1beta1.Listener, state *state.ListenerState, ref gwv1alpha2.ParentReference, route *K8sRoute) bool {
	if state.Status.Ready.HasError() {
		return false
	}

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(listener.Name, ref.SectionName)
	if !allowed {
		return false
	}

	if !routeKindIsAllowedForListener(common.SupportedKindsFor(listener.Protocol), route) {
		if must {
			route.RouteState.BindFailed(errors.NewBindErrorRouteKind("route kind not allowed for listener"), ref)
		}
		return false
	}

	allowed, err := routeAllowedForListenerNamespaces(ctx, namespace, listener.AllowedRoutes, route, b.Client)
	if err != nil {
		route.RouteState.BindFailed(fmt.Errorf("error checking listener namespaces: %w", err), ref)
		return false
	}
	if !allowed {
		if must {
			route.RouteState.BindFailed(errors.NewBindErrorListenerNamespacePolicy("route not allowed because of listener namespace policy"), ref)
		}
		return false
	}

	if !route.matchesHostname(listener.Hostname) {
		if must {
			route.RouteState.BindFailed(errors.NewBindErrorHostnameMismatch("route does not match listener hostname"), ref)
		}
		return false
	}

	// check if the route is valid, if not, then return a status about it being rejected
	if !route.RouteState.ResolutionErrors.Empty() {
		route.RouteState.BindFailed(errors.NewBindErrorRouteInvalid("route is in an invalid state and cannot bind"), ref)
		return false
	}

	route.RouteState.Bound(ref)
	return true
}

// routeAllowedForListenerNamespaces determines whether the route is allowed
// to bind to the Gateway based on the AllowedRoutes namespace selectors.
func routeAllowedForListenerNamespaces(ctx context.Context, gatewayNS string, allowedRoutes *gwv1beta1.AllowedRoutes, route *K8sRoute, c gatewayclient.Client) (bool, error) {
	var namespaceSelector *gwv1beta1.RouteNamespaces
	if allowedRoutes != nil {
		// check gateway namespace
		namespaceSelector = allowedRoutes.Namespaces
	}

	// set default if namespace selector is nil
	from := gwv1beta1.NamespacesFromSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gwv1beta1.NamespacesFromAll:
		return true, nil
	case gwv1beta1.NamespacesFromSame:
		return gatewayNS == route.GetNamespace(), nil
	case gwv1beta1.NamespacesFromSelector:
		namespaceSelector, err := metav1.LabelSelectorAsSelector(namespaceSelector.Selector)
		if err != nil {
			return false, fmt.Errorf("error parsing label selector: %w", err)
		}

		// retrieve the route's namespace and determine whether selector matches
		namespace, err := c.GetNamespace(ctx, types.NamespacedName{Name: route.GetNamespace()})
		if err != nil {
			return false, fmt.Errorf("error retrieving namespace for route: %w", err)
		}

		return namespaceSelector.Matches(toNamespaceSet(namespace.GetName(), namespace.GetLabels())), nil
	}
	return false, nil
}

func routeKindIsAllowedForListener(kinds []gwv1beta1.RouteGroupKind, route *K8sRoute) bool {
	if kinds == nil {
		return true
	}

	gvk := route.GroupVersionKind()
	for _, kind := range kinds {
		group := gwv1beta1.GroupName
		if kind.Group != nil && *kind.Group != "" {
			group = string(*kind.Group)
		}
		if string(kind.Kind) == gvk.Kind && group == gvk.Group {
			return true
		}
	}

	return false
}

func toNamespaceSet(name string, labels map[string]string) klabels.Labels {
	// If namespace label is not set, implicitly insert it to support older Kubernetes versions
	if labels[NamespaceNameLabel] == name {
		// Already set, avoid copies
		return klabels.Set(labels)
	}
	// First we need a copy to not modify the underlying object
	ret := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		ret[k] = v
	}
	ret[NamespaceNameLabel] = name
	return klabels.Set(ret)
}

func routeMatchesListener(listenerName gwv1beta1.SectionName, routeSectionName *gwv1alpha2.SectionName) (can bool, must bool) {
	if routeSectionName == nil {
		return true, false
	}
	return string(listenerName) == string(*routeSectionName), true
}
