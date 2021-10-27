package controllers

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	"github.com/hashicorp/go-hclog"
)

const (
	gatewayClassFinalizer = "gateway-exists-finalizer.networking.x-k8s.io"
)

// GatewayClassReconciler reconciles a GatewayClass object
type GatewayClassReconciler struct {
	Client         gatewayclient.Client
	Log            hclog.Logger
	ControllerName string
	Manager        reconciler.ReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.With("gatewayClass", req.NamespacedName)

	gc, err := r.Client.GetGatewayClass(ctx, req.NamespacedName)
	if err != nil {
		logger.Error("failed to get gateway class", "error", err)
		return ctrl.Result{}, err
	}

	if gc == nil {
		// we've been deleted clean up cached resources
		err = r.Manager.DeleteGatewayClass(ctx, req.NamespacedName.Name)
		return ctrl.Result{}, err
	}

	if string(gc.Spec.ControllerName) != r.ControllerName {
		// no-op if we don't manage this gateway class
		return ctrl.Result{}, nil
	}

	if !gc.ObjectMeta.DeletionTimestamp.IsZero() {
		// we have a deletion, ensure we're not in use
		used, err := r.Client.GatewayClassInUse(ctx, gc)
		if err != nil {
			logger.Error("failed to check if the gateway class is still in use", "error", err)
			return ctrl.Result{}, err
		}
		if used {
			// requeue as to not block the reconciliation loop
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// remove finalizer
		if _, err := r.Client.RemoveFinalizer(ctx, gc, gatewayClassFinalizer); err != nil {
			logger.Error("error removing gateway class finalizer", "error", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// we're creating or updating
	updated, err := r.Client.EnsureFinalizer(ctx, gc, gatewayClassFinalizer)
	if err != nil {
		logger.Error("error adding gateway class finalizer", "error", err)
		return ctrl.Result{}, err
	}
	if updated {
		// since we've updated the finalizers, returning here will enqueue another event
		return ctrl.Result{}, nil
	}
	if err := r.Manager.UpsertGatewayClass(ctx, gc); err != nil {
		logger.Error("error upserting gateway class", "error", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.GatewayClass{}).
		Complete(gatewayclient.NewRequeueingMiddleware(r.Log, r))
}
