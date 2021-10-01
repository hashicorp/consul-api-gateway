package reconciler

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func setParentStatus(status gw.RouteStatus, conditionType gw.RouteConditionType, statuses ...gw.RouteParentStatus) gw.RouteStatus {
	parents := status.Parents
	for _, toSet := range statuses {
		parentFound := false
		for idx, parent := range parents {
			if parent.ParentRef == toSet.ParentRef && parent.Controller == toSet.Controller {
				parentFound = true
				conditions := []metav1.Condition{}
				for _, condition := range parent.Conditions {
					updated := false

					if condition.Type == string(conditionType) {
						for _, updatedCondition := range toSet.Conditions {
							if updatedCondition.Type == string(conditionType) {
								conditions = append(conditions, updateCondition(condition, updatedCondition))
								updated = true
								// just update with the first condition of this type
								break
							}
						}
					}

					if !updated {
						conditions = append(conditions, condition)
					}
				}
				parent.Conditions = conditions
				parents[idx] = parent
			}
		}
		if !parentFound {
			parents = append(parents, toSet)
		}
	}

	return gw.RouteStatus{
		Parents: parents,
	}
}

func setAdmittedStatus(status gw.RouteStatus, statuses ...gw.RouteParentStatus) gw.RouteStatus {
	return setParentStatus(status, gw.ConditionRouteAdmitted, statuses...)
}

func setResolvedRefsStatus(status gw.RouteStatus, statuses ...gw.RouteParentStatus) gw.RouteStatus {
	return setParentStatus(status, gw.ConditionRouteResolvedRefs, statuses...)
}

func clearParentStatus(controllerName, namespace string, status gw.RouteStatus, namespacedName types.NamespacedName) gw.RouteStatus {
	parents := []gw.RouteParentStatus{}
	for _, parent := range status.Parents {
		gatewayName, isGateway := referencesGateway(namespace, parent.ParentRef)
		if isGateway && gatewayName == namespacedName && parent.Controller == gw.GatewayController(controllerName) {
			continue
		}
		parents = append(parents, parent)
	}

	return gw.RouteStatus{
		Parents: parents,
	}
}

// updateCondition returns the latest condition minus the transition timestamps
// it should only be used if you know the condition Type values are the same
func updateCondition(current, updated metav1.Condition) metav1.Condition {
	if current.ObservedGeneration > updated.ObservedGeneration {
		return current
	}

	if current.ObservedGeneration != updated.ObservedGeneration {
		return updated
	}
	if current.Message != updated.Message {
		return updated
	}
	if current.Reason != updated.Reason {
		return updated
	}
	if current.Status != updated.Status {
		return updated
	}
	return current
}
