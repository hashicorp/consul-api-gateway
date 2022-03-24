package reconciler

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Binder struct {
	Client        gatewayclient.Client
	Gateway       *gw.Gateway
	Listener      gw.Listener
	ListenerState *ListenerState
}

func (b *Binder) CanBind(ctx context.Context, route *K8sRoute) (bool, error) {
	for _, ref := range route.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(route.GetNamespace(), ref); isGateway {
			expected := utils.NamespacedName(b.Gateway)
			if expected == namespacedName {
				canBind, err := b.canBind(ctx, ref, route)
				if err != nil {
					return false, err
				}
				if canBind {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (b *Binder) canBind(ctx context.Context, ref gw.ParentRef, route *K8sRoute) (bool, error) {
	if b.ListenerState.Status.Ready.HasError() {
		return false, nil
	}

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(b.Listener.Name, ref.SectionName)
	if allowed {
		if !routeKindIsAllowedForListener(supportedKindsFor(b.Listener.Protocol), route) {
			if must {
				return false, NewBindErrorRouteKind("route kind not allowed for listener")
			}
			return false, nil
		}

		allowed, err := routeAllowedForListenerNamespaces(ctx, b.Gateway.Namespace, b.Listener.AllowedRoutes, route, b.Client)
		if err != nil {
			return false, fmt.Errorf("error checking listener namespaces: %w", err)
		}
		if !allowed {
			if must {
				return false, NewBindErrorListenerNamespacePolicy("route not allowed because of listener namespace policy")
			}
			return false, nil
		}

		if !route.MatchesHostname(b.Listener.Hostname) {
			if must {
				return false, NewBindErrorHostnameMismatch("route does not match listener hostname")
			}
			return false, nil
		}

		// check if the route is valid, if not, then return a status about it being rejected
		if !route.IsValid() {
			return false, NewBindErrorRouteInvalid("route is in an invalid state and cannot bind")
		}
		return true, nil
	}

	return false, nil
}
