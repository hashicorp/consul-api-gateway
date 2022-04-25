package status

import (
	"errors"
	"sort"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	rerrors "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RouteStatuses map[string]*RouteStatus

func (r RouteStatuses) Statuses(controllerName string, generation int64) []gw.RouteParentStatus {
	statuses := []gw.RouteParentStatus{}
	for ref, status := range r {
		statuses = append(statuses, gw.RouteParentStatus{
			ParentRef:      common.ParseParent(ref),
			ControllerName: gw.GatewayController(controllerName),
			Conditions:     status.Conditions(generation),
		})
	}
	return statuses
}

func (r RouteStatuses) mergedStatus(routeStatus gw.RouteStatus, controllerName string, generation int64) gw.RouteStatus {
	return gw.RouteStatus{
		Parents: sortParents(append(filterParentStatuses(routeStatus, controllerName), r.Statuses(controllerName, generation)...)),
	}
}

func (r RouteStatuses) NeedsUpdate(routeStatus gw.RouteStatus, controllerName string, generation int64) (gw.RouteStatus, bool) {
	currentStatus := gw.RouteStatus{Parents: sortParents(routeStatus.Parents)}
	updatedStatus := r.mergedStatus(routeStatus, controllerName, generation)
	return updatedStatus, !RouteStatusEqual(currentStatus, updatedStatus)
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

func sortParents(parents []gw.RouteParentStatus) []gw.RouteParentStatus {
	for _, parent := range parents {
		sort.SliceStable(parent.Conditions, func(i, j int) bool {
			return common.AsJSON(parent.Conditions[i]) < common.AsJSON(parent.Conditions[j])
		})
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return common.AsJSON(parents[i]) < common.AsJSON(parents[j])
	})
	return parents
}

func (r RouteStatuses) BindFailed(resolutionErrors *service.ResolutionErrors, err error, id string) {
	routeStatus, statusFound := r[id]
	if !statusFound {
		routeStatus = &RouteStatus{}
	}
	var bindError rerrors.BindError
	if errors.As(err, &bindError) {
		switch bindError.Kind() {
		case rerrors.BindErrorTypeHostnameMismatch:
			routeStatus.Accepted.ListenerHostnameMismatch = err
		case rerrors.BindErrorTypeListenerNamespacePolicy:
			routeStatus.Accepted.ListenerNamespacePolicy = err
		case rerrors.BindErrorTypeRouteKind:
			routeStatus.Accepted.InvalidRouteKind = err
		case rerrors.BindErrorTypeRouteInvalid:
			routeStatus.Accepted.BindError = err
		}
	} else {
		routeStatus.Accepted.BindError = err
	}
	// set resolution errors - we can do this here because
	// a route with resolution errors will always fail to bind
	errorType, err := resolutionErrors.Flatten()
	switch errorType {
	case service.GenericResolutionErrorType:
		routeStatus.ResolvedRefs.Errors = err
	case service.ConsulServiceResolutionErrorType:
		routeStatus.ResolvedRefs.ConsulServiceNotFound = err
	case service.K8sServiceResolutionErrorType:
		routeStatus.ResolvedRefs.ServiceNotFound = err
	case service.RefNotPermittedErrorType:
		routeStatus.ResolvedRefs.RefNotPermitted = err
	}

	r[id] = routeStatus
}

func (r RouteStatuses) Bound(id string) {
	// clear out any existing errors on our statuses
	if routeStatus, statusFound := r[id]; statusFound {
		routeStatus.Accepted = RouteAcceptedStatus{}
		routeStatus.ResolvedRefs = RouteResolvedRefsStatus{}
	} else {
		r[id] = &RouteStatus{}
	}
}

func (r RouteStatuses) Remove(id string) {
	delete(r, id)
}
