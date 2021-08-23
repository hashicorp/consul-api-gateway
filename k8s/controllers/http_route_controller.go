package controllers

import (
	"context"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/polar/k8s/reconciler"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Manager *reconciler.GatewayReconcileManager

	image string
}

//+kubebuilder:rbac:groups=polar.hashicorp.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=polar.hashicorp.com,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=polar.hashicorp.com,resources=gateways/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("gateway", req.NamespacedName)

	route := &gateway.HTTPRoute{}
	err := r.Get(ctx, req.NamespacedName, route)
	// If the gateway object has been deleted (and we get an IsNotFound
	// error), we need to stop the associated deployment.
	if k8serrors.IsNotFound(err) {
		r.Manager.DeleteRoute(req.NamespacedName)
		// TODO stop deployment
		return ctrl.Result{}, nil
	} else if err != nil {
		r.Log.Error(err, "failed to get HTTPRoute", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}

	r.Log.Info("retrieved", "name", route.Name, "ns", route.Namespace)
	r.Manager.UpsertHTTPRoute(route)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For()
		For(&gateway.HTTPRoute{}).
		Complete(r)
}
