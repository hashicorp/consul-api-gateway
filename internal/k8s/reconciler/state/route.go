package state

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

// RouteState holds ephemeral state for routes
type RouteState struct {
	// TODO: make this able to be marshaled
	References       service.RouteRuleReferenceMap
	ResolutionErrors *service.ResolutionErrors
	ParentStatuses   status.RouteStatuses
}

func NewRouteState() *RouteState {
	return &RouteState{
		References:       make(service.RouteRuleReferenceMap),
		ResolutionErrors: service.NewResolutionErrors(),
		ParentStatuses:   make(status.RouteStatuses),
	}
}
