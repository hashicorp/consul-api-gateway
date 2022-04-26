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

func NewRouteState() *RouteState {
	return &RouteState{
		References:       make(service.RouteRuleReferenceMap),
		ResolutionErrors: service.NewResolutionErrors(),
		ParentStatuses:   make(status.RouteStatuses),
	}
}

func (r *RouteState) BindFailed(err error, ref gw.ParentRef) {
	r.ParentStatuses.BindFailed(r.ResolutionErrors, err, common.AsJSON(ref))
}

func (r *RouteState) Bound(ref gw.ParentRef) {
	r.ParentStatuses.Bound(common.AsJSON(ref))
}

func (r *RouteState) Remove(ref gw.ParentRef) {
	r.ParentStatuses.Remove(common.AsJSON(ref))
}
