package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	"github.com/hashicorp/go-hclog"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	Client         gatewayclient.Client
	Log            hclog.Logger
	ControllerName string
	Manager        reconciler.ReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.With("route", req.NamespacedName)

	route, err := r.Client.GetHTTPRoute(ctx, req.NamespacedName)
	if err != nil {
		logger.Error("failed to get http route", "error", err)
		return ctrl.Result{}, err
	}

	if route == nil {
		// clean up cached resources
		r.Manager.DeleteRoute(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	managed, err := r.Client.IsManagedRoute(ctx, route.Spec.CommonRouteSpec, route.Namespace, r.ControllerName)
	if err != nil {
		logger.Error("error validating gateway usage for route", "error", err)
		return ctrl.Result{}, err
	}
	if !managed {
		// we're not managing this route (potentially reference got removed on an update)
		// ensure it's cleaned up
		r.Manager.DeleteRoute(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// let the route get upserted so long as there's a single gateway we control
	// that it's managed by -- the underlying reconciliation code will handle the
	// validation of gateway attachment
	r.Manager.UpsertHTTPRoute(route)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.HTTPRoute{}).
		Complete(r)
}
