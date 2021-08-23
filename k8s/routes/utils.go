package routes

import (
	"reflect"

	klabels "k8s.io/apimachinery/pkg/labels"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
const NamespaceNameLabel = "kubernetes.io/metadata.name"

func toNamespaceSet(name string, labels map[string]string) klabels.Labels {
	// If namespace label is not set, implicitly insert it to support older Kubernetes versions
	if labels[NamespaceNameLabel] == name {
		// Already set, avoid copies
		return klabels.Set(labels)
	}
	// First we need a copy to not modify the underlying object
	ret := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		ret[k] = v
	}
	ret[NamespaceNameLabel] = name
	return klabels.Set(ret)
}

func parentRefsEqual(a, b gw.ParentRef) bool {
	return compareStrPtrs(a.Group, b.Group) &&
		compareStrPtrs(a.Kind, b.Kind) &&
		compareStrPtrs(a.Namespace, b.Namespace) &&
		compareStrPtrs(a.SectionName, b.Namespace) &&
		compareStrPtrs(a.Scope, a.Scope) &&
		a.Name == b.Name
}

func compareStrPtrs(a, b *string) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	return *a == *b
}

func getRouteStatusPtr(route interface{}) *gw.RouteStatus {
	val := reflect.ValueOf(route).Elem()
	var statusVal reflect.Value
	for i := 0; i < val.NumField(); i++ {
		if val.Type().Field(i).Name == "Status" {
			statusVal = val.Field(i)
		}
	}

	if !statusVal.IsValid() {
		return nil
	}
	var routeStatusVal reflect.Value
	for i := 0; i < statusVal.NumField(); i++ {
		if statusVal.Type().Field(i).Name == "RouteStatus" {
			routeStatusVal = statusVal.Field(i)
		}
	}
	if !routeStatusVal.IsValid() {
		return nil
	}

	return routeStatusVal.Addr().Interface().(*gw.RouteStatus)
}
