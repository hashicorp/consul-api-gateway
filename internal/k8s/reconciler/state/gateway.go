package state

import (
	"errors"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// GatewayState holds ephemeral state for gateways
type GatewayState struct {
	Status       status.GatewayStatus
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
	Status     status.ListenerStatus
}

func (l *ListenerState) Valid() bool {
	routeCount := atomic.LoadInt32(&l.RouteCount)
	if l.Protocol == gw.TCPProtocolType {
		if routeCount > 1 {
			return false
		}
	}
	return l.Status.Valid()
}

func (l *ListenerState) getStatus(generation int64) gw.ListenerStatus {
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
		SupportedKinds: common.SupportedKindsFor(l.Protocol),
		AttachedRoutes: routeCount,
		Conditions:     l.Status.Conditions(generation),
	}
}
