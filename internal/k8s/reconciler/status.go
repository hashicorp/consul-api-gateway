package reconciler

import (
	"encoding/json"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type conditionSetKey struct {
	parentRef     string
	controller    gw.GatewayController
	conditionType string
}

type conditionSet map[conditionSetKey]metav1.Condition

func parentStatusesToCondtionSet(statuses []gw.RouteParentStatus) conditionSet {
	set := make(map[conditionSetKey]metav1.Condition)
	for _, status := range statuses {
		for _, condition := range status.Conditions {
			// note that the gateway spec mentions we should only have a single entry
			// for each Condition Type per controller/parent, so any controllers
			// violating that will get clobbered by this set operation
			set[conditionSetKey{
				parentRef:     stringifyParentRef(status.ParentRef),
				controller:    status.Controller,
				conditionType: condition.Type,
			}] = condition
		}
	}
	return conditionSet(set)
}

func (s conditionSet) toParentStatuses() []gw.RouteParentStatus {
	statuses := make(map[conditionSetKey]gw.RouteParentStatus)
	for key, condition := range s {
		// construct a key without the conditionType so that we can
		// merge all statuses back together by their parent/controller
		// references
		parentKey := conditionSetKey{
			parentRef:  key.parentRef,
			controller: key.controller,
		}
		status, found := statuses[parentKey]
		if !found {
			status = gw.RouteParentStatus{
				ParentRef:  parseParentRef(key.parentRef),
				Controller: key.controller,
			}
		}
		status.Conditions = append(status.Conditions, condition)
		statuses[parentKey] = status
	}

	// sort all conditions for stability
	parents := []gw.RouteParentStatus{}
	for _, status := range statuses {
		parents = append(parents, status)
	}

	// now sort all the parent references
	return sortParents(parents)
}

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

func setParentStatus(status gw.RouteStatus, conditionType gw.RouteConditionType, statuses ...gw.RouteParentStatus) gw.RouteStatus {
	parentSet := parentStatusesToCondtionSet(status.Parents)
	for _, status := range statuses {
		for _, condition := range status.Conditions {
			conditionKey := conditionSetKey{
				parentRef:     stringifyParentRef(status.ParentRef),
				controller:    status.Controller,
				conditionType: condition.Type,
			}
			// add or potentially override whatever is in the set
			parentSet[conditionKey] = updateCondition(parentSet[conditionKey], condition)
		}
	}

	return gw.RouteStatus{
		Parents: parentSet.toParentStatuses(),
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
		Parents: sortParents(parents),
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
