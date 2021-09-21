package controllers

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/hashicorp/consul-api-gateway/k8s/utils"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// PodReconciler reconciles Pod objects
type PodReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	Tracker *utils.PodTracker
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("pod", req.NamespacedName)

	pod := &core.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if k8serrors.IsNotFound(err) {
		// If the pod is deleted, we have no way of getting gateway information so just no-op
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "failed to get Pod")
		return ctrl.Result{}, err
	}
	gwName, managed := managedGateway(pod.ObjectMeta.Labels)
	if !managed {
		return ctrl.Result{}, nil
	}
	found := &gateway.Gateway{}
	err = r.Get(ctx, types.NamespacedName{Name: gwName, Namespace: req.Namespace}, found)
	if k8serrors.IsNotFound(err) {
		// the gateway object is gone, which means we've been deleted, ensure we're not
		// following the status updates any more
		r.Tracker.DeleteStatus(pod)
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "failed to get Gateway")
		return ctrl.Result{}, err
	}
	if found.CreationTimestamp.After(pod.CreationTimestamp.Time) {
		// we have an old pod from a previous deployment that has the same name
		// as our current gateway deployment, ignore it
		return ctrl.Result{}, nil
	}
	if !found.DeletionTimestamp.IsZero() {
		// we're deleting the gateway, clean up the tracker
		r.Tracker.DeleteStatus(pod)
		return ctrl.Result{}, nil
	}

	conditions := mapGatewayConditions(pod.Generation, pod.Status)
	if r.Tracker.UpdateStatus(pod, conditions) {
		log.Info("gateway deployment pod status updated", "conditions", conditions)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&core.Pod{}).
		Complete(r)
}

func managedGateway(labels map[string]string) (string, bool) {
	managedBy, ok := labels["managedBy"]
	if !ok || managedBy != "consul-api-gateway" {
		return "", false
	}
	name, ok := labels["name"]
	if !ok {
		return "", false
	}
	return name, true
}

func mapGatewayConditions(generation int64, status core.PodStatus) []meta.Condition {
	// TODO: real state tracking for fsm

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
	switch status.Phase {
	case core.PodPending:
		return mapStatusPending(generation, status)
	case core.PodRunning:
		return mapStatusRunning(generation, status)
	case core.PodSucceeded:
		// this should never happen, occurs when the pod terminates
		// with a 0 status code, consider this a failed deployment
		fallthrough
	case core.PodFailed:
		// we have a failed deployment, set the status accordingly
		// for now we just consider the pods unschedulable.
		// TODO: more fine-grained
		return []meta.Condition{{
			Type:               string(gateway.GatewayConditionScheduled),
			Reason:             string(gateway.GatewayReasonNoResources),
			Status:             meta.ConditionFalse,
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}}
	default: // Unknown pod status
		// we don't have a known pod status, just consider this unreconciled
		return []meta.Condition{{
			Type:               string(gateway.GatewayConditionScheduled),
			Reason:             string(gateway.GatewayReasonNotReconciled),
			Status:             meta.ConditionFalse,
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}}
	}
}

func mapStatusPending(generation int64, status core.PodStatus) []meta.Condition {
	for _, condition := range status.Conditions {
		if condition.Type == core.PodScheduled && condition.Status == core.ConditionFalse &&
			strings.Contains(condition.Reason, "Unschedulable") {
			return []meta.Condition{{
				Type:               string(gateway.GatewayConditionScheduled),
				Reason:             string(gateway.GatewayReasonNoResources),
				Status:             meta.ConditionFalse,
				ObservedGeneration: generation,
				LastTransitionTime: meta.Now(),
			}}
		}
		if condition.Type == core.PodScheduled && condition.Status == core.ConditionTrue {
			return []meta.Condition{{
				Type:               string(gateway.GatewayConditionScheduled),
				Reason:             string(gateway.GatewayReasonScheduled),
				Status:             meta.ConditionTrue,
				ObservedGeneration: generation,
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
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}}
}

func mapStatusRunning(generation int64, status core.PodStatus) []meta.Condition {
	for _, condition := range status.Conditions {
		if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
			return []meta.Condition{{
				Type:               string(gateway.GatewayConditionScheduled),
				Reason:             string(gateway.GatewayReasonScheduled),
				Status:             meta.ConditionTrue,
				ObservedGeneration: generation,
				LastTransitionTime: meta.Now(),
			}, {
				Type:               string(gateway.GatewayConditionReady),
				Reason:             string(gateway.GatewayReasonReady),
				Status:             meta.ConditionTrue,
				ObservedGeneration: generation,
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
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}, {
		Type:               string(gateway.GatewayConditionReady),
		Reason:             string(gateway.GatewayReasonListenersNotReady),
		Status:             meta.ConditionFalse,
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}}
}
