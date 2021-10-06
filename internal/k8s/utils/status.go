package utils

import (
	"strings"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func MapGatewayConditionsFromPod(pod *core.Pod) []meta.Condition {
	if pod == nil {
		return []meta.Condition{{
			Type:               string(gateway.GatewayConditionScheduled),
			Reason:             string(gateway.GatewayReasonNotReconciled),
			Status:             meta.ConditionFalse,
			LastTransitionTime: meta.Now(),
		}}
	}
	// Pending: The pod has been accepted by the Kubernetes system, but one or more of the
	// container images has not been created. This includes time before being scheduled as
	// well as time spent downloading images over the network, which could take a while.
	// Running: The pod has been bound to a node, and all of the containers have been created.
	// At least one container is still running, or is in the process of starting or restarting.
	// Succeeded: All containers in the pod have terminated in success, and will not be restarted.
	// Failed: All containers in the pod have terminated, and at least one container has
	// terminated in failure. The container either exited with non-zero status or was terminated
	// by the system.
	// Unknown: For some reason the state of the pod could not be obtained, typically due to an
	// error in communicating with the host of the pod.
	switch pod.Status.Phase {
	case core.PodPending:
		return mapStatusPending(pod)
	case core.PodRunning:
		return mapStatusRunning(pod)
	case core.PodSucceeded:
		// this should never happen, occurs when the pod terminates
		// with a 0 status code, consider this a failed deployment
		fallthrough
	case core.PodFailed:
		// we have a failed deployment, set the status accordingly
		// for now we just consider the pods unschedulable.
		// TODO: look into more fine-grained failure reasons
		return []meta.Condition{{
			Type:               string(gateway.GatewayConditionScheduled),
			Reason:             string(gateway.GatewayReasonNoResources),
			Status:             meta.ConditionFalse,
			ObservedGeneration: pod.Generation,
			LastTransitionTime: meta.Now(),
		}}
	default: // Unknown pod status
		// we don't have a known pod status, just consider this unreconciled
		return []meta.Condition{{
			Type:               string(gateway.GatewayConditionScheduled),
			Reason:             string(gateway.GatewayReasonNotReconciled),
			Status:             meta.ConditionFalse,
			ObservedGeneration: pod.Generation,
			LastTransitionTime: meta.Now(),
		}}
	}
}

func mapStatusPending(pod *core.Pod) []meta.Condition {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodScheduled && condition.Status == core.ConditionFalse &&
			strings.Contains(condition.Reason, "Unschedulable") {
			return []meta.Condition{{
				Type:               string(gateway.GatewayConditionScheduled),
				Reason:             string(gateway.GatewayReasonNoResources),
				Status:             meta.ConditionFalse,
				ObservedGeneration: pod.Generation,
				LastTransitionTime: meta.Now(),
			}}
		}
		if condition.Type == core.PodScheduled && condition.Status == core.ConditionTrue {
			return []meta.Condition{{
				Type:               string(gateway.GatewayConditionScheduled),
				Reason:             string(gateway.GatewayReasonScheduled),
				Status:             meta.ConditionTrue,
				ObservedGeneration: pod.Generation,
				LastTransitionTime: meta.Now(),
			}}
		}
	}
	// if no conditions exist, or we haven't found a specific above condition, just default
	// to not reconciled
	return []meta.Condition{{
		Type:               string(gateway.GatewayConditionScheduled),
		Reason:             string(gateway.GatewayReasonNotReconciled),
		Status:             meta.ConditionFalse,
		ObservedGeneration: pod.Generation,
		LastTransitionTime: meta.Now(),
	}}
}

func mapStatusRunning(pod *core.Pod) []meta.Condition {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
			return []meta.Condition{{
				Type:               string(gateway.GatewayConditionScheduled),
				Reason:             string(gateway.GatewayReasonScheduled),
				Status:             meta.ConditionTrue,
				ObservedGeneration: pod.Generation,
				LastTransitionTime: meta.Now(),
			}, {
				Type:               string(gateway.GatewayConditionReady),
				Reason:             string(gateway.GatewayReasonReady),
				Status:             meta.ConditionTrue,
				ObservedGeneration: pod.Generation,
				LastTransitionTime: meta.Now(),
			}}
		}
	}
	// if no conditions exist, or we haven't found a specific above condition, just default
	// to no listeners ready
	return []meta.Condition{{
		Type:               string(gateway.GatewayConditionScheduled),
		Reason:             string(gateway.GatewayReasonScheduled),
		Status:             meta.ConditionTrue,
		ObservedGeneration: pod.Generation,
		LastTransitionTime: meta.Now(),
	}, {
		Type:               string(gateway.GatewayConditionReady),
		Reason:             string(gateway.GatewayReasonListenersNotReady),
		Status:             meta.ConditionFalse,
		ObservedGeneration: pod.Generation,
		LastTransitionTime: meta.Now(),
	}}
}
