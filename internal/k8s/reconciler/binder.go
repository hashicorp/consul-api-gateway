package reconciler

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel = "kubernetes.io/metadata.name"
)

type Binder struct {
	Client       gatewayclient.Client
	Gateway      *gw.Gateway
	GatewayState *state.GatewayState
}

func NewBinder(client gatewayclient.Client, gateway *gw.Gateway, state *state.GatewayState) *Binder {
	return &Binder{
		Client:       client,
		Gateway:      gateway,
		GatewayState: state,
	}
}

func (b *Binder) Bind(ctx context.Context, route *K8sRoute) []string {
	boundListeners := []string{}
	for _, ref := range route.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(route.GetNamespace(), ref); isGateway {
			expected := utils.NamespacedName(b.Gateway)
			if expected == namespacedName {
				for i, listener := range b.Gateway.Spec.Listeners {
					state := b.GatewayState.Listeners[i]
					if b.canBind(ctx, listener, state, ref, route) {
						state.Routes[route.ID()] = route.resolve(b.GatewayState.ConsulNamespace, b.Gateway, listener)
						boundListeners = append(boundListeners, string(listener.Name))
					}
				}
			}
		}
	}

	return boundListeners
}

func (b *Binder) canBind(ctx context.Context, listener gw.Listener, state *state.ListenerState, ref gw.ParentRef, route *K8sRoute) bool {
	if state.Status.Ready.HasError() {
		return false
	}

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(listener.Name, ref.SectionName)
	if allowed {
		if !routeKindIsAllowedForListener(common.SupportedKindsFor(listener.Protocol), route) {
			if must {
				route.RouteState.BindFailed(errors.NewBindErrorRouteKind("route kind not allowed for listener"), ref)
			}
			return false
		}

		allowed, err := routeAllowedForListenerNamespaces(ctx, b.Gateway.Namespace, listener.AllowedRoutes, route, b.Client)
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

		route.RouteState.ParentStatuses.Bound(common.AsJSON(ref))
		return true
	}

	return false
}

// routeAllowedForListenerNamespaces determines whether the route is allowed
// to bind to the Gateway based on the AllowedRoutes namespace selectors.
func routeAllowedForListenerNamespaces(ctx context.Context, gatewayNS string, allowedRoutes *gw.AllowedRoutes, route *K8sRoute, c gatewayclient.Client) (bool, error) {
	var namespaceSelector *gw.RouteNamespaces
	if allowedRoutes != nil {
		// check gateway namespace
		namespaceSelector = allowedRoutes.Namespaces
	}

	// set default if namespace selector is nil
	from := gw.NamespacesFromSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gw.NamespacesFromAll:
		return true, nil
	case gw.NamespacesFromSame:
		return gatewayNS == route.GetNamespace(), nil
	case gw.NamespacesFromSelector:
		namespaceSelector, err := meta.LabelSelectorAsSelector(namespaceSelector.Selector)
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

func routeKindIsAllowedForListener(kinds []gw.RouteGroupKind, route *K8sRoute) bool {
	if kinds == nil {
		return true
	}

	gvk := route.GroupVersionKind()
	for _, kind := range kinds {
		group := gw.GroupName
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

func routeMatchesListener(listenerName gw.SectionName, sectionName *gw.SectionName) (bool, bool) {
	if sectionName == nil {
		return true, false
	}
	return listenerName == *sectionName, true
}
