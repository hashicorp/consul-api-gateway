package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	Context        context.Context
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
	logger := r.Log.With("http-route", req.NamespacedName)

	route, err := r.Client.GetHTTPRoute(ctx, req.NamespacedName)
	if err != nil {
		logger.Error("failed to get http route", "error", err)
		return ctrl.Result{}, err
	}

	if route == nil {
		// clean up cached resources
		err := r.Manager.DeleteHTTPRoute(ctx, req.NamespacedName)
		return ctrl.Result{}, err
	}

	// let the route get upserted so long as there's a single gateway we control
	// that it's managed by -- the underlying reconciliation code will handle the
	// validation of gateway attachment
	err = r.Manager.UpsertHTTPRoute(ctx, route)
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.HTTPRoute{}).
		Watches(
			&source.Kind{Type: &gateway.ReferencePolicy{}},
			handler.EnqueueRequestsFromMapFunc(r.referencePolicyToRouteRequests),
		).
		Watches(
			&source.Kind{Type: &corev1.Service{}},
			handler.EnqueueRequestsFromMapFunc(r.serviceToRouteRequests),
		).
		Complete(gatewayclient.NewRequeueingMiddleware(r.Log, r))
}

// serviceToRouteRequests builds a list of HTTPRoutes that need to be reconciled
// based on changes to a Service
func (r *HTTPRouteReconciler) serviceToRouteRequests(object client.Object) []reconcile.Request {
	service := object.(*corev1.Service)

	routes := r.getRoutesAffectedByService(service)
	var requests []reconcile.Request

	for _, route := range routes {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		})
	}

	return requests
}

// getRoutesAffectedByService retrieves all HTTPRoutes potentially impacted
// by the Service being modified. This is done by filtering to HTTPRoutes that
// have a backendRef matching the Service's namespace and name.
func (r *HTTPRouteReconciler) getRoutesAffectedByService(service *corev1.Service) []gateway.HTTPRoute {
	var matches []gateway.HTTPRoute

	routes, err := r.Client.GetHTTPRoutes(r.Context)
	if err != nil {
		r.Log.Error("error fetching routes", err)
		return matches
	}

	// Return any routes that have a backend reference to service
	for _, route := range routes {
	nextRoute:
		for _, rule := range route.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				// The BackendRef may or may not specify a namespace, defaults to route's namespace
				refNamespace := route.Namespace
				if ref.Namespace != nil && *ref.Namespace != "" {
					refNamespace = string(*ref.Namespace)
				}

				// If this BackendRef matches the service namespace + name, then this HTTPRoute
				// is affected. No need to check other refs, skip ahead to next HTTPRoute.
				if refNamespace == service.Namespace && ref.Name == gateway.ObjectName(service.Name) {
					matches = append(matches, route)
					break nextRoute
				}
			}
		}
	}

	return matches
}

// For UpdateEvents which contain both a new and old object, this transformation
// function is run on both objects and both sets of Requests are enqueued.
//
// This is needed to reconcile any objects matched by both current and prior
// state in case a ReferencePolicy has been modified to revoke permission from a
// namespace or to a service
//
// It may be possible to improve performance here by filtering Routes by
// BackendRefs selectable by the To fields, but currently we just revalidate
// all Routes allowed in the From Namespaces
func (r *HTTPRouteReconciler) referencePolicyToRouteRequests(object client.Object) []reconcile.Request {
	refPolicy := object.(*gateway.ReferencePolicy)

	routes := r.getRoutesAffectedByReferencePolicy(refPolicy)
	requests := []reconcile.Request{}

	for _, route := range routes {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		})
	}

	return requests
}

// getRoutesAffectedByReferencePolicy retrieves all HTTPRoutes potentially impacted
// by the ReferencePolicy being modified. Currently, this is unfiltered and so returns
// all HTTPRoutes in the namespace referenced by the ReferencePolicy.
func (r *HTTPRouteReconciler) getRoutesAffectedByReferencePolicy(refPolicy *gateway.ReferencePolicy) []gateway.HTTPRoute {
	var matches []gateway.HTTPRoute

	for _, from := range refPolicy.Spec.From {
		// TODO: search by from.Group and from.Kind instead of assuming this ReferencePolicy references a HTTPRoute
		routes, err := r.Client.GetHTTPRoutesInNamespace(r.Context, string(from.Namespace))
		if err != nil {
			r.Log.Error("error fetching routes", err)
			return matches
		}

		matches = append(matches, routes...)
	}

	return matches
}
