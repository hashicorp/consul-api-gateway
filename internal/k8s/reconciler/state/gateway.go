package state

import (
	"errors"

	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
)

// GatewayState holds ephemeral state for gateways
type GatewayState struct {
	ConsulNamespace string
	Status          status.GatewayStatus
	PodReady        bool
	ServiceReady    bool
	Generation      int64
	Addresses       []string

	Listeners []*ListenerState
}

func InitialGatewayState(g *gwv1beta1.Gateway) *GatewayState {
	state := &GatewayState{
		Generation: g.GetGeneration(),
	}
	for _, listener := range g.Spec.Listeners {
		state.Listeners = append(state.Listeners, &ListenerState{
			Name:     listener.Name,
			Protocol: listener.Protocol,
			Routes:   make(map[string]core.ResolvedRoute),
		})
	}
	return state
}

func (g *GatewayState) GetStatus(gateway *gwv1beta1.Gateway) gwv1beta1.GatewayStatus {
	listenerStatuses := []gwv1beta1.ListenerStatus{}
	for _, state := range g.Listeners {
		listenerStatuses = append(listenerStatuses, state.getStatus(g.Generation))
	}

	conditions := g.Status.Conditions(g.Generation)

	// prefer to not update to not mess up timestamps
	if status.ListenerStatusesEqual(listenerStatuses, gateway.Status.Listeners) {
		listenerStatuses = gateway.Status.Listeners
	}
	if status.ConditionsEqual(conditions, gateway.Status.Conditions) {
		conditions = gateway.Status.Conditions
	}

	ipType := gwv1beta1.IPAddressType
	addresses := make([]gwv1beta1.GatewayAddress, 0, len(g.Addresses))
	for _, address := range g.Addresses {
		addresses = append(addresses, gwv1beta1.GatewayAddress{
			Type:  &ipType,
			Value: address,
		})
	}

	return gwv1beta1.GatewayStatus{
		Addresses:  addresses,
		Conditions: conditions,
		Listeners:  listenerStatuses,
	}
}

// ListenerState holds ephemeral state for listeners
type ListenerState struct {
	Routes   map[string]core.ResolvedRoute
	Protocol gwv1beta1.ProtocolType
	Name     gwv1beta1.SectionName
	TLS      core.TLSParams
	Status   status.ListenerStatus
}

func (l *ListenerState) Valid() bool {
	routeCount := len(l.Routes)
	if l.Protocol == gwv1beta1.TCPProtocolType {
		if routeCount > 1 {
			return false
		}
	}
	return l.Status.Valid()
}

func (l *ListenerState) getStatus(generation int64) gwv1beta1.ListenerStatus {
	routeCount := len(l.Routes)
	if l.Protocol == gwv1beta1.TCPProtocolType {
		if routeCount > 1 {
			l.Status.Conflicted.RouteConflict = errors.New("only a single TCP route can be bound to a TCP listener")
		} else {
			l.Status.Conflicted.RouteConflict = nil
		}
	}
	return gwv1beta1.ListenerStatus{
		Name:           l.Name,
		SupportedKinds: common.SupportedKindsFor(l.Protocol),
		AttachedRoutes: int32(routeCount),
		Conditions:     l.Status.Conditions(generation),
	}
}
