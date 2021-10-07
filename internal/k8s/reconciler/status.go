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
		sort.SliceStable(parent.Conditions, func(i, j int) bool {
			return asJSON(parent.Conditions[i]) < asJSON(parent.Conditions[j])
		})
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return asJSON(parents[i]) < asJSON(parents[j])
	})
	return parents
}

func asJSON(item interface{}) string {
	data, err := json.Marshal(item)
	if err != nil {
		// everything passed to this internally should be
		// serializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
	return string(data)
}

func parseParent(stringified string) gw.ParentRef {
	var ref gw.ParentRef
	if err := json.Unmarshal([]byte(stringified), &ref); err != nil {
		// everything passed to this internally should be
		// deserializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
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
	if asJSON(a.ParentRef) != asJSON(b.ParentRef) {
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
