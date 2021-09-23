package controllers

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/k8s/reconciler"
	"github.com/hashicorp/go-hclog"
)

var (
	errGatewayClassInUse = errors.New("gateway class is still in use")
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
		logger.Error("failed to get GatewayClass", "error", err)
		return ctrl.Result{}, err
	}

	if gc == nil {
		// we've been deleted clean up cached resources
		r.Manager.DeleteGatewayClass(req.NamespacedName.Name)
		return ctrl.Result{}, nil
	}

	if string(gc.Spec.Controller) != r.ControllerName {
		// no-op if we don't manage this gateway class
		return ctrl.Result{}, nil
	}

	if !gc.ObjectMeta.DeletionTimestamp.IsZero() {
		// we have a deletion, ensure we're not in use
		used, err := r.Client.GatewayClassInUse(ctx, gc)
		if err != nil {
			logger.Error("failed to check if the gateway class is still in use", "error", err, "name", gc.Name)
			return ctrl.Result{}, err
		}
		if used {
			return ctrl.Result{}, errGatewayClassInUse
		}
		// remove finalizer
		if _, err := r.Client.RemoveFinalizer(ctx, gc, gatewayClassFinalizer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// we're creating or updating
	updated, err := r.Client.EnsureFinalizer(ctx, gc, gatewayClassFinalizer)
	if err != nil {
		return ctrl.Result{}, err
	}
	if updated {
		// requeue for versioning
		return ctrl.Result{Requeue: true}, nil
	}
	// this validation is used for setting the gateway class accepted status
	valid, err := r.Client.IsValidGatewayClass(ctx, gc)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Manager.UpsertGatewayClass(gc, valid)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager, scheme *runtime.Scheme) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.GatewayClass{}).
		Complete(r)
}
