package service

import (
	"encoding/json"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RouteRule struct {
	HTTPRule *gw.HTTPRouteRule
	TCPRule  *gw.TCPRouteRule
}

func NewRouteRule(rule interface{}) RouteRule {
	r := RouteRule{}
	switch routeRule := rule.(type) {
	case *gw.HTTPRouteRule:
		r.HTTPRule = routeRule
	case *gw.TCPRouteRule:
		r.TCPRule = routeRule
	}
	return r
}

type RouteRuleReferenceMap map[RouteRule][]ResolvedReference

func (r RouteRuleReferenceMap) MarshalJSON() ([]byte, error) {
	data := map[string][]ResolvedReference{}

	for k, v := range r {
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		data[string(key)] = v
	}
	return json.Marshal(data)
}

func (r *RouteRuleReferenceMap) UnmarshalJSON(b []byte) error {
	*r = map[RouteRule][]ResolvedReference{}
	data := map[string][]ResolvedReference{}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	for k, v := range data {
		rule := RouteRule{}
		if err := json.Unmarshal([]byte(k), &rule); err != nil {
			return err
		}
		(*r)[rule] = v
	}
	return nil
}

func (r RouteRuleReferenceMap) Add(rule RouteRule, resolved ResolvedReference) {
	refs, found := r[rule]
	if found {
		r[rule] = append(refs, resolved)
		return
	}
	r[rule] = []ResolvedReference{resolved}
}
