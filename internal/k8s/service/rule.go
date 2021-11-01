package service

import (
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RouteRule struct {
	HTTPRule *gw.HTTPRouteRule
	TCPRule  *gw.TCPRouteRule
	TLSRule  *gw.TLSRouteRule
	UDPRule  *gw.UDPRouteRule
}

func NewRouteRule(rule interface{}) RouteRule {
	r := RouteRule{}
	switch routeRule := rule.(type) {
	case *gw.HTTPRouteRule:
		r.HTTPRule = routeRule
	case *gw.TCPRouteRule:
		r.TCPRule = routeRule
	case *gw.UDPRouteRule:
		r.UDPRule = routeRule
	case *gw.TLSRouteRule:
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
