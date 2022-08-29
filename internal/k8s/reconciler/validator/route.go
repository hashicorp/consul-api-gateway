package validator

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

// all kubernetes routes implement the following two interfaces
type Route interface {
	client.Object
	schema.ObjectKind
}

type RouteValidator struct {
	client   gatewayclient.Client
	resolver service.BackendResolver
}

func NewRouteValidator(resolver service.BackendResolver, client gatewayclient.Client) *RouteValidator {
	return &RouteValidator{
		resolver: resolver,
		client:   client,
	}
}

func (r *RouteValidator) Validate(ctx context.Context, state *state.RouteState, route Route) error {
	switch route := route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return r.validateHTTPRoute(ctx, state, route)
	case *gwv1alpha2.TCPRoute:
		return r.validateTCPRoute(ctx, state, route)
	}
	return nil
}

func (r *RouteValidator) validateHTTPRoute(ctx context.Context, state *state.RouteState, route *gwv1alpha2.HTTPRoute) error {
	for _, httpRule := range route.Spec.Rules {
		rule := httpRule
		routeRule := service.NewRouteRule(&rule)
		for _, backendRef := range rule.BackendRefs {
			ref := backendRef

			allowed, err := routeAllowedForBackendRef(ctx, route, ref.BackendRef, r.client)
			if err != nil {
				return err
			} else if !allowed {
				msg := fmt.Sprintf("Cross-namespace routing not allowed without matching ReferencePolicy for Service %q", getNamespacedName(ref.Name, ref.Namespace, route.GetNamespace()))
				state.ResolutionErrors.Add(service.NewRefNotPermittedError(msg))
				continue
			}

			reference, err := r.resolver.Resolve(ctx, route.GetNamespace(), ref.BackendObjectReference)
			if err != nil {
				var resolutionError service.ResolutionError
				if !errors.As(err, &resolutionError) {
					return err
				}
				state.ResolutionErrors.Add(resolutionError)
				continue
			}
			reference.Reference.Set(&ref)
			state.References.Add(routeRule, *reference)
		}
	}
	return nil
}

func (r *RouteValidator) validateTCPRoute(ctx context.Context, state *state.RouteState, route *gwv1alpha2.TCPRoute) error {
	if len(route.Spec.Rules) != 1 {
		err := service.NewResolutionError("a single tcp rule is required")
		state.ResolutionErrors.Add(err)
		return nil
	}

	rule := route.Spec.Rules[0]

	if len(rule.BackendRefs) != 1 {
		err := service.NewResolutionError("a single backendRef per tcp rule is required")
		state.ResolutionErrors.Add(err)
		return nil
	}

	routeRule := service.NewRouteRule(rule)

	ref := rule.BackendRefs[0]

	allowed, err := routeAllowedForBackendRef(ctx, route, ref, r.client)
	if err != nil {
		return err
	} else if !allowed {
		msg := fmt.Sprintf("Cross-namespace routing not allowed without matching ReferencePolicy for Service %q", getNamespacedName(ref.Name, ref.Namespace, route.GetNamespace()))
		state.ResolutionErrors.Add(service.NewRefNotPermittedError(msg))
		return nil
	}

	reference, err := r.resolver.Resolve(ctx, route.GetNamespace(), ref.BackendObjectReference)
	if err != nil {
		var resolutionError service.ResolutionError
		if !errors.As(err, &resolutionError) {
			return err
		}
		state.ResolutionErrors.Add(resolutionError)
		return nil
	}

	reference.Reference.Set(&ref)
	state.References.Add(routeRule, *reference)
	return nil
}

// routeAllowedForBackendRef determines whether the route is allowed
// for the backend either by being in the same namespace or by having
// an applicable ReferenceGrant in the same namespace as the backend.
//
// TODO This func is currently called once for each backendRef on a route and results
//   in fetching ReferenceGrants more than we technically have to in some cases
func routeAllowedForBackendRef(ctx context.Context, route Route, backendRef gwv1alpha2.BackendRef, c gatewayclient.Client) (bool, error) {
	fromNS := route.GetNamespace()
	fromGK := metav1.GroupKind{
		Group: route.GroupVersionKind().Group,
		Kind:  route.GroupVersionKind().Kind,
	}

	toName := string(backendRef.Name)
	toNS := ""
	if backendRef.Namespace != nil {
		toNS = string(*backendRef.Namespace)
	}

	// Kind should default to Service if not set
	// https://github.com/kubernetes-sigs/gateway-api/blob/ef773194892636ea8ecbb2b294daf771d4dd5009/apis/v1alpha2/object_reference_types.go#L105
	toGK := metav1.GroupKind{Kind: "Service"}
	if backendRef.Group != nil {
		toGK.Group = string(*backendRef.Group)
	}
	if backendRef.Kind != nil {
		toGK.Kind = string(*backendRef.Kind)
	}

	return referenceAllowed(ctx, fromGK, fromNS, toGK, toNS, toName, c)
}
