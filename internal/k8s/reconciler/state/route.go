package state

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// RouteState holds ephemeral state for routes
type RouteState struct {
	// TODO: make this able to be marshaled
	References       service.RouteRuleReferenceMap
	ResolutionErrors *service.ResolutionErrors
	ParentStatuses   status.RouteStatuses
}

func (r *RouteState) Statuses(controllerName string, generation int64) []gw.RouteParentStatus {
	statuses := []gw.RouteParentStatus{}
	for ref, status := range r.ParentStatuses {
		statuses = append(statuses, gw.RouteParentStatus{
			ParentRef:      common.ParseParent(ref),
			ControllerName: gw.GatewayController(controllerName),
			Conditions:     status.Conditions(generation),
		})
	}
	return statuses
}
