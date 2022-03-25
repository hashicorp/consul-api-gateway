package reconciler

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
						atomic.AddInt32(&state.RouteCount, 1)
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
				route.bindFailed(errors.NewBindErrorRouteKind("route kind not allowed for listener"), ref)
			}
			return false
		}

		allowed, err := routeAllowedForListenerNamespaces(ctx, b.Gateway.Namespace, listener.AllowedRoutes, route, b.Client)
		if err != nil {
			route.bindFailed(fmt.Errorf("error checking listener namespaces: %w", err), ref)
			return false
		}
		if !allowed {
			if must {
				route.bindFailed(errors.NewBindErrorListenerNamespacePolicy("route not allowed because of listener namespace policy"), ref)
			}
			return false
		}

		if !route.MatchesHostname(listener.Hostname) {
			if must {
				route.bindFailed(errors.NewBindErrorHostnameMismatch("route does not match listener hostname"), ref)
			}
			return false
		}

		// check if the route is valid, if not, then return a status about it being rejected
		if !route.RouteState.ResolutionErrors.Empty() {
			route.bindFailed(errors.NewBindErrorRouteInvalid("route is in an invalid state and cannot bind"), ref)
			return false
		}

		route.RouteState.ParentStatuses.Bound(common.AsJSON(ref))
		return true
	}

	return false
}
