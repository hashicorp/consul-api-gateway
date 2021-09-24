package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/k8s/reconciler"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	ControllerName string
	Manager        *reconciler.GatewayReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("route", req.NamespacedName)

	route := &gateway.HTTPRoute{}
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		if k8serrors.IsNotFound(err) {
			// clean up cached resources
			r.Manager.DeleteRoute(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get HTTPRoute")
		return ctrl.Result{}, err
	}

	managed, err := isManagedRoute(ctx, r.Client, route.Spec.CommonRouteSpec, r.ControllerName)
	if err != nil {
		logger.Error(err, "error validating gateway usage for route")
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

func isManagedRoute(ctx context.Context, client client.Client, spec gateway.CommonRouteSpec, controllerName string) (bool, error) {
	for _, ref := range spec.ParentRefs {
		gw := &gateway.Gateway{}
		name := types.NamespacedName{Name: ref.Name}
		if ref.Namespace != nil {
			name.Namespace = string(*ref.Namespace)
		}
		if err := client.Get(ctx, name, gw); err != nil {
			return false, fmt.Errorf("failed to get gateway: %w", err)
		}

		gc, err := gatewayClassForGateway(ctx, client, gw)
		if err != nil {
			return false, fmt.Errorf("failed to get gateway class: %w", err)
		}

		if string(gc.Spec.Controller) == controllerName {
			return true, err
		}
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.HTTPRoute{}).
		Complete(r)
}
