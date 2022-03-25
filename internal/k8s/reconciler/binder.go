package reconciler

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Binder struct {
	Client        gatewayclient.Client
	Gateway       *K8sGateway
	Listener      gw.Listener
	ListenerState *state.ListenerState
}

func (b *Binder) Bind(ctx context.Context, route *K8sRoute) bool {
	for _, ref := range route.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(route.GetNamespace(), ref); isGateway {
			expected := utils.NamespacedName(b.Gateway)
			if expected == namespacedName {
				return b.canBind(ctx, ref, route)
			}
		}
	}
	return false
}

func (b *Binder) canBind(ctx context.Context, ref gw.ParentRef, route *K8sRoute) bool {
	if b.ListenerState.Status.Ready.HasError() {
		return false
	}

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(b.Listener.Name, ref.SectionName)
	if allowed {
		if !routeKindIsAllowedForListener(common.SupportedKindsFor(b.Listener.Protocol), route) {
			if must {
				route.bindFailed(errors.NewBindErrorRouteKind("route kind not allowed for listener"), b.Gateway)
			}
			return false
		}

		allowed, err := routeAllowedForListenerNamespaces(ctx, b.Gateway.Namespace, b.Listener.AllowedRoutes, route, b.Client)
		if err != nil {
			route.bindFailed(fmt.Errorf("error checking listener namespaces: %w", err), b.Gateway)
			return false
		}
		if !allowed {
			if must {
				route.bindFailed(errors.NewBindErrorListenerNamespacePolicy("route not allowed because of listener namespace policy"), b.Gateway)
			}
			return false
		}

		if !route.MatchesHostname(b.Listener.Hostname) {
			if must {
				route.bindFailed(errors.NewBindErrorHostnameMismatch("route does not match listener hostname"), b.Gateway)
			}
			return false
		}

		// check if the route is valid, if not, then return a status about it being rejected
		if !route.RouteState.ResolutionErrors.Empty() {
			route.bindFailed(errors.NewBindErrorRouteInvalid("route is in an invalid state and cannot bind"), b.Gateway)
			return false
		}

		route.bound(b.Gateway)
		return true
	}

	return false
}
