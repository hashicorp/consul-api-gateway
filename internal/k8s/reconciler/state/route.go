// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package state

import (
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

// RouteState holds ephemeral state for routes
type RouteState struct {
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

func (r *RouteState) BindFailed(err error, ref gwv1alpha2.ParentReference) {
	r.ParentStatuses.BindFailed(r.ResolutionErrors, err, common.AsJSON(ref))
}

func (r *RouteState) Bound(ref gwv1alpha2.ParentReference) {
	r.ParentStatuses.Bound(common.AsJSON(ref))
}

func (r *RouteState) Remove(ref gwv1alpha2.ParentReference) {
	r.ParentStatuses.Remove(common.AsJSON(ref))
}
