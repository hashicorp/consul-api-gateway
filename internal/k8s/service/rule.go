package service

import (
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RouteRule struct {
	HTTPRule *gwv1alpha2.HTTPRouteRule
	TCPRule  *gwv1alpha2.TCPRouteRule
	TLSRule  *gwv1alpha2.TLSRouteRule
	UDPRule  *gwv1alpha2.UDPRouteRule
}

func NewRouteRule(rule interface{}) RouteRule {
	r := RouteRule{}
	switch routeRule := rule.(type) {
	case *gwv1alpha2.HTTPRouteRule:
		r.HTTPRule = routeRule
	case *gwv1alpha2.TCPRouteRule:
		r.TCPRule = routeRule
	case *gwv1alpha2.UDPRouteRule:
		r.UDPRule = routeRule
	case *gwv1alpha2.TLSRouteRule:
		r.TLSRule = routeRule
	}
	return r
}

type RouteRuleReferenceMap map[RouteRule][]ResolvedReference

func (r RouteRuleReferenceMap) Add(rule RouteRule, resolved ResolvedReference) {
	refs, found := r[rule]
	if found {
		r[rule] = append(refs, resolved)
		return
	}
	r[rule] = []ResolvedReference{resolved}
}
