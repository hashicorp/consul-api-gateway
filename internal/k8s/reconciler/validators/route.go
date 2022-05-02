package validators

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
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

func (r *RouteValidator) Validate(ctx context.Context, route Route) (*state.RouteState, error) {
	state := &state.RouteState{
		ResolutionErrors: service.NewResolutionErrors(),
		References:       make(service.RouteRuleReferenceMap),
		ParentStatuses:   make(status.RouteStatuses),
	}

	switch route := route.(type) {
	case *gw.HTTPRoute:
		return r.validateHTTPRoute(ctx, state, route)
	case *gw.TCPRoute:
		return r.validateTCPRoute(ctx, state, route)
	}

	return nil, nil
}

func (r *RouteValidator) validateHTTPRoute(ctx context.Context, state *state.RouteState, route *gw.HTTPRoute) (*state.RouteState, error) {
	for _, httpRule := range route.Spec.Rules {
		rule := httpRule
		routeRule := service.NewRouteRule(&rule)
		for _, backendRef := range rule.BackendRefs {
			ref := backendRef

			allowed, err := routeAllowedForBackendRef(ctx, route, ref.BackendRef, r.client)
			if err != nil {
				return nil, err
			} else if !allowed {
				msg := fmt.Sprintf("Cross-namespace routing not allowed without matching ReferencePolicy for Service %q", getServiceID(ref.Name, ref.Namespace, route.GetNamespace()))
				state.ResolutionErrors.Add(service.NewRefNotPermittedError(msg))
				continue
			}

			reference, err := r.resolver.Resolve(ctx, route.GetNamespace(), ref.BackendObjectReference)
			if err != nil {
				var resolutionError service.ResolutionError
				if !errors.As(err, &resolutionError) {
					return nil, err
				}
				state.ResolutionErrors.Add(resolutionError)
				continue
			}
			reference.Reference.Set(&ref)
			state.References.Add(routeRule, *reference)
		}
	}
	return state, nil
}

func (r *RouteValidator) validateTCPRoute(ctx context.Context, state *state.RouteState, route *gw.TCPRoute) (*state.RouteState, error) {
	if len(route.Spec.Rules) != 1 {
		err := service.NewResolutionError("a single tcp rule is required")
		state.ResolutionErrors.Add(err)
		return state, nil
	}

	rule := route.Spec.Rules[0]

	if len(rule.BackendRefs) != 1 {
		err := service.NewResolutionError("a single backendRef per tcp rule is required")
		state.ResolutionErrors.Add(err)
		return state, nil
	}

	routeRule := service.NewRouteRule(rule)

	ref := rule.BackendRefs[0]

	allowed, err := routeAllowedForBackendRef(ctx, route, ref, r.client)
	if err != nil {
		return nil, err
	} else if !allowed {
		msg := fmt.Sprintf("Cross-namespace routing not allowed without matching ReferencePolicy for Service %q", getServiceID(ref.Name, ref.Namespace, route.GetNamespace()))
		state.ResolutionErrors.Add(service.NewRefNotPermittedError(msg))
		return state, nil
	}

	reference, err := r.resolver.Resolve(ctx, route.GetNamespace(), ref.BackendObjectReference)
	if err != nil {
		var resolutionError service.ResolutionError
		if !errors.As(err, &resolutionError) {
			return nil, err
		}
		state.ResolutionErrors.Add(resolutionError)
		return state, nil
	}

	reference.Reference.Set(&ref)
	state.References.Add(routeRule, *reference)
	return state, nil
}

func getServiceID(name gw.ObjectName, namespace *gw.Namespace, parentNamespace string) string {
	serviceID := fmt.Sprintf("%s/%s", name, parentNamespace)
	if namespace != nil {
		serviceID = fmt.Sprintf("%s/%s", name, *namespace)
	}
	return serviceID
}

// routeAllowedForBackendRef determines whether the route is allowed
// for the backend either by being in the same namespace or by having
// an applicable ReferencePolicy in the same namespace as the backend.
//
// TODO This func is currently called once for each backendRef on a route and results
//   in fetching ReferencePolicies more than we technically have to in some cases
func routeAllowedForBackendRef(ctx context.Context, route Route, backendRef gw.BackendRef, c gatewayclient.Client) (bool, error) {
	backendNamespace := ""
	if backendRef.Namespace != nil {
		backendNamespace = string(*backendRef.Namespace)
	}

	// Allow if route and backend are in the same namespace
	if backendNamespace == "" || route.GetNamespace() == backendNamespace {
		return true, nil
	}

	// Allow if ReferencePolicy present for route + backend combination
	refPolicies, err := c.GetReferencePoliciesInNamespace(ctx, backendNamespace)
	if err != nil || len(refPolicies) == 0 {
		return false, err
	}

	for _, refPolicy := range refPolicies {
		// Check for a From that applies to the route
		validFrom := false
		for _, from := range refPolicy.Spec.From {
			// If this policy allows the group, kind and namespace for this route
			if route.GroupVersionKind().Group == string(from.Group) &&
				route.GroupVersionKind().Kind == string(from.Kind) &&
				route.GetNamespace() == string(from.Namespace) {
				validFrom = true
				break
			}
		}

		// If this ReferencePolicy has no applicable From, no need to check for a To
		if !validFrom {
			continue
		}

		// Backend group should default to empty string if not set
		backendRefGroup := gw.Group("")
		if backendRef.Group != nil {
			backendRefGroup = *backendRef.Group
		}

		// Backend kind should default to Service if not set
		// TODO Should we default to Service here or go look up the kind from K8s API
		//   See https://github.com/kubernetes-sigs/gateway-api/issues/1092
		backendRefKind := gw.Kind("Service")
		if backendRef.Kind != nil {
			backendRefKind = *backendRef.Kind
		}

		// Check for a To that applies to the backendRef
		for _, to := range refPolicy.Spec.To {
			// If this policy allows the group, kind, and name for this backend
			if to.Group == backendRefGroup &&
				to.Kind == backendRefKind &&
				(to.Name == nil || *to.Name == backendRef.Name) {
				return true, nil
			}
		}
	}

	return false, err
}
