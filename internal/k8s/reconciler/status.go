package reconciler

import (
	"encoding/json"
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func sortParents(parents []gw.RouteParentStatus) []gw.RouteParentStatus {
	for _, parent := range parents {
		sort.SliceStable(parent.Conditions, sortConditions(parent.Conditions))
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return compareJSON(parents[i]) < compareJSON(parents[j])
	})
	return parents
}

func sortConditions(conditions []metav1.Condition) func(int, int) bool {
	return func(i, j int) bool {
		return compareJSON(conditions[i]) < compareJSON(conditions[j])
	}
}

func compareJSON(item interface{}) string {
	data, _ := json.Marshal(item)
	return string(data)
}

func stringifyParentRef(ref gw.ParentRef) string {
	data, _ := json.Marshal(ref)
	return string(data)
}

func parseParentRef(stringified string) gw.ParentRef {
	var ref gw.ParentRef
	_ = json.Unmarshal([]byte(stringified), &ref)
	return ref
}

func conditionEqual(a, b metav1.Condition) bool {
	if a.Type != b.Type ||
		a.ObservedGeneration != b.ObservedGeneration ||
		a.Status != b.Status ||
		a.Reason != b.Reason ||
		a.Message != b.Message {
		return false
	}
	return true
}

func conditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		// we have a different number of conditions, so they aren't the same
		return false
	}

	for i, newCondition := range a {
		if !conditionEqual(newCondition, b[i]) {
			return false
		}
	}
	return true
}

func listenerStatusEqual(a, b gw.ListenerStatus) bool {
	if a.Name != b.Name {
		return false
	}
	if !reflect.DeepEqual(a.SupportedKinds, b.SupportedKinds) {
		return false
	}
	if a.AttachedRoutes != b.AttachedRoutes {
		return false
	}
	return conditionsEqual(a.Conditions, b.Conditions)
}

func listenerStatusesEqual(a, b []gw.ListenerStatus) bool {
	if len(a) != len(b) {
		// we have a different number of conditions, so they aren't the same
		return false
	}
	for i, newStatus := range a {
		if !listenerStatusEqual(newStatus, b[i]) {
			return false
		}
	}
	return true
}

func parentStatusEqual(a, b gw.RouteParentStatus) bool {
	if a.Controller != b.Controller {
		return false
	}
	if stringifyParentRef(a.ParentRef) != stringifyParentRef(b.ParentRef) {
		return false
	}

	return conditionsEqual(a.Conditions, b.Conditions)
}

func routeStatusEqual(a, b gw.RouteStatus) bool {
	if len(a.Parents) != len(b.Parents) {
		return false
	}

	for i, oldParent := range a.Parents {
		if !parentStatusEqual(oldParent, b.Parents[i]) {
			return false
		}
	}
	return true
}

func gatewayStatusEqual(a, b gw.GatewayStatus) bool {
	if !conditionsEqual(a.Conditions, b.Conditions) {
		return false
	}

	if !listenerStatusesEqual(a.Listeners, b.Listeners) {
		return false
	}

	return true
}
