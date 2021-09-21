package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/hashicorp/consul-api-gateway/k8s/utils"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	gwName, managed := utils.IsManagedGateway(pod.ObjectMeta.Labels)
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

	conditions := utils.MapGatewayConditionsFromPod(pod)
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
