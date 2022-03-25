package reconciler

import (
	"errors"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// GatewayState holds ephemeral state for gateways
type GatewayState struct {
	Status       GatewayStatus
	PodReady     bool
	ServiceReady bool
	Generation   int64
	Addresses    []string

	Listeners []*ListenerState
}

func InitialGatewayState(g *gw.Gateway) *GatewayState {
	state := &GatewayState{
		Generation: g.GetGeneration(),
	}
	for _, listener := range g.Spec.Listeners {
		state.Listeners = append(state.Listeners, &ListenerState{
			Name:     listener.Name,
			Protocol: listener.Protocol,
		})
	}
	return state
}

func (g *GatewayState) GetStatus(gateway *gw.Gateway) gw.GatewayStatus {
	listenerStatuses := []gw.ListenerStatus{}
	for _, state := range g.Listeners {
		listenerStatuses = append(listenerStatuses, state.GetStatus(g.Generation))
	}

	conditions := g.Status.Conditions(g.Generation)

	// prefer to not update to not mess up timestamps
	if listenerStatusesEqual(listenerStatuses, gateway.Status.Listeners) {
		listenerStatuses = gateway.Status.Listeners
	}
	if conditionsEqual(conditions, gateway.Status.Conditions) {
		conditions = gateway.Status.Conditions
	}

	ipType := gw.IPAddressType
	addresses := make([]gw.GatewayAddress, 0, len(g.Addresses))
	for _, address := range g.Addresses {
		addresses = append(addresses, gw.GatewayAddress{
			Type:  &ipType,
			Value: address,
		})
	}

	return gw.GatewayStatus{
		Addresses:  addresses,
		Conditions: conditions,
		Listeners:  listenerStatuses,
	}
}

// ListenerState holds ephemeral state for listeners
type ListenerState struct {
	RouteCount int32
	Protocol   gw.ProtocolType
	Name       gw.SectionName
	TLS        core.TLSParams
	Status     ListenerStatus
}

func (l *ListenerState) ValidWithProtocol(protocol gw.ProtocolType) bool {
	routeCount := atomic.LoadInt32(&l.RouteCount)
	if protocol == gw.TCPProtocolType {
		if routeCount > 1 {
			return false
		}
	}
	return l.Status.Valid()
}

func (l *ListenerState) GetStatus(generation int64) gw.ListenerStatus {
	routeCount := atomic.LoadInt32(&l.RouteCount)
	if l.Protocol == gw.TCPProtocolType {
		if routeCount > 1 {
			l.Status.Conflicted.RouteConflict = errors.New("only a single TCP route can be bound to a TCP listener")
		} else {
			l.Status.Conflicted.RouteConflict = nil
		}
	}
	return gw.ListenerStatus{
		Name:           l.Name,
		SupportedKinds: supportedKindsFor(l.Protocol),
		AttachedRoutes: routeCount,
		Conditions:     l.Status.Conditions(generation),
	}
}

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
