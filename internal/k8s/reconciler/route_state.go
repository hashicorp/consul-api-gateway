package reconciler

import (
	"errors"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// RouteState holds ephemeral state for routes
type RouteState struct {
	// TODO: make this able to be marshaled
	References       service.RouteRuleReferenceMap
	ResolutionErrors *service.ResolutionErrors
	ParentStatuses   RouteStatuses
}

func (r *RouteState) bindFailed(err error, id string) {
	status, statusFound := r.ParentStatuses[id]
	if !statusFound {
		status = &RouteStatus{}
	}
	var bindError BindError
	if errors.As(err, &bindError) {
		switch bindError.Kind() {
		case BindErrorTypeHostnameMismatch:
			status.Accepted.ListenerHostnameMismatch = err
		case BindErrorTypeListenerNamespacePolicy:
			status.Accepted.ListenerNamespacePolicy = err
		case BindErrorTypeRouteKind:
			status.Accepted.InvalidRouteKind = err
		case BindErrorTypeRouteInvalid:
			status.Accepted.BindError = err
		}
	} else {
		status.Accepted.BindError = err
	}
	// set resolution errors - we can do this here because
	// a route with resolution errors will always fail to bind
	errorType, err := r.ResolutionErrors.Flatten()
	switch errorType {
	case service.GenericResolutionErrorType:
		status.ResolvedRefs.Errors = err
	case service.ConsulServiceResolutionErrorType:
		status.ResolvedRefs.ConsulServiceNotFound = err
	case service.K8sServiceResolutionErrorType:
		status.ResolvedRefs.ServiceNotFound = err
	}

	r.ParentStatuses[id] = status
}

func (r *RouteState) bound(id string) {
	// clear out any existing errors on our statuses
	if status, statusFound := r.ParentStatuses[id]; statusFound {
		status.Accepted = RouteAcceptedStatus{}
		status.ResolvedRefs = RouteResolvedRefsStatus{}
	} else {
		r.ParentStatuses[id] = &RouteStatus{}
	}
}

func (r *RouteState) remove(id string) {
	delete(r.ParentStatuses, id)
}

func (r *RouteState) Statuses(controllerName string, generation int64) []gw.RouteParentStatus {
	statuses := []gw.RouteParentStatus{}
	for ref, status := range r.ParentStatuses {
		statuses = append(statuses, gw.RouteParentStatus{
			ParentRef:      parseParent(ref),
			ControllerName: gw.GatewayController(controllerName),
			Conditions:     status.Conditions(generation),
		})
	}
	return statuses
}

func filterParentStatuses(routeStatus gw.RouteStatus, controllerName string) []gw.RouteParentStatus {
	filtered := []gw.RouteParentStatus{}
	for _, status := range routeStatus.Parents {
		if status.ControllerName != gw.GatewayController(controllerName) {
			filtered = append(filtered, status)
			continue
		}
	}
	return filtered
}

func mergedStatus(routeStatus gw.RouteStatus, controllerName string, generation int64, state *RouteState) gw.RouteStatus {
	return gw.RouteStatus{
		Parents: sortParents(append(filterParentStatuses(routeStatus, controllerName), state.Statuses(controllerName, generation)...)),
	}
}

func needsStatusUpdate(routeStatus gw.RouteStatus, controllerName string, generation int64, state *RouteState) (gw.RouteStatus, bool) {
	currentStatus := gw.RouteStatus{Parents: sortParents(routeStatus.Parents)}
	updatedStatus := mergedStatus(routeStatus, controllerName, generation, state)
	return updatedStatus, !routeStatusEqual(currentStatus, updatedStatus)
}
