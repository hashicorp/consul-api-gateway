package status

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
)

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

func ConditionsEqual(a, b []metav1.Condition) bool {
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
	return ConditionsEqual(a.Conditions, b.Conditions)
}

func ListenerStatusesEqual(a, b []gwv1beta1.ListenerStatus) bool {
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
	if common.AsJSON(a.ParentRef) != common.AsJSON(b.ParentRef) {
		return false
	}

	return ConditionsEqual(a.Conditions, b.Conditions)
}

func RouteStatusEqual(a, b gwv1alpha2.RouteStatus) bool {
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

func GatewayStatusEqual(a, b gwv1beta1.GatewayStatus) bool {
	if !ConditionsEqual(a.Conditions, b.Conditions) {
		return false
	}

	if !ListenerStatusesEqual(a.Listeners, b.Listeners) {
		return false
	}

	return true
}
