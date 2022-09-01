package reconciler

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func routeMatchesListenerHostname(listenerHostname *gwv1beta1.Hostname, hostnames []gwv1alpha2.Hostname) bool {
	if listenerHostname == nil || len(hostnames) == 0 {
		return true
	}

	for _, name := range hostnames {
		if hostnamesMatch(name, *listenerHostname) {
			return true
		}
	}
	return false
}

func hostnamesMatch(a gwv1alpha2.Hostname, b gwv1beta1.Hostname) bool {
	if a == "" || a == "*" || b == "" || b == "*" {
		// any wildcard always matches
		return true
	}

	if strings.HasPrefix(string(a), "*.") || strings.HasPrefix(string(b), "*.") {
		aLabels, bLabels := strings.Split(string(a), "."), strings.Split(string(b), ".")
		if len(aLabels) != len(bLabels) {
			return false
		}

		for i := 1; i < len(aLabels); i++ {
			if !strings.EqualFold(aLabels[i], bLabels[i]) {
				return false
			}
		}
		return true
	}

	return string(a) == string(b)
}

func sortParents(parents []gwv1alpha2.RouteParentStatus) []gwv1alpha2.RouteParentStatus {
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

func parseParent(stringified string) gwv1alpha2.ParentReference {
	var ref gwv1alpha2.ParentReference
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

func listenerStatusEqual(a, b gwv1beta1.ListenerStatus) bool {
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

func listenerStatusesEqual(a, b []gwv1beta1.ListenerStatus) bool {
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

func parentStatusEqual(a, b gwv1alpha2.RouteParentStatus) bool {
	if a.ControllerName != b.ControllerName {
		return false
	}
	if asJSON(a.ParentRef) != asJSON(b.ParentRef) {
		return false
	}

	return conditionsEqual(a.Conditions, b.Conditions)
}

func routeStatusEqual(a, b gwv1alpha2.RouteStatus) bool {
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

func gatewayStatusEqual(a, b gwv1beta1.GatewayStatus) bool {
	if !conditionsEqual(a.Conditions, b.Conditions) {
		return false
	}

	if !listenerStatusesEqual(a.Listeners, b.Listeners) {
		return false
	}

	return true
}

type gwObjectName interface {
	gwv1beta1.ObjectName | gwv1alpha2.ObjectName
}

type gwNamespace interface {
	gwv1beta1.Namespace | gwv1alpha2.Namespace
}

// getNamespacedName returns types.NamespacedName defaulted to a parent
// namespace in the case where the provided namespace is nil.
func getNamespacedName[O gwObjectName, N gwNamespace](name O, namespace *N, parentNamespace string) types.NamespacedName {
	if namespace != nil {
		return types.NamespacedName{Namespace: string(*namespace), Name: string(name)}
	}
	return types.NamespacedName{Namespace: parentNamespace, Name: string(name)}
}
