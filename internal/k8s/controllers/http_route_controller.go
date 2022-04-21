package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
		Complete(gatewayclient.NewRequeueingMiddleware(r.Log, r))
}

// For UpdateEvents which contain both a new and old object, this transformation
// function is run on both objects and both sets of Requests are enqueued.
//
// This is needed to reconcile any objects matched by  both current and prior
// state in case a ReferencePolicy has been modified to revoke permission from a
// namespace or to a service
//
// It may be possible to improve performance here by filtering Routes by
// BackendRefs selectable by the To fields, but currently we just revalidate
// all Routes allowed in the From Namespaces
func (r *HTTPRouteReconciler) referencePolicyToRouteRequests(object client.Object) []reconcile.Request {
	// TODO: Is there a safer way I could typecheck this with
	// object.GetObjectKind() or something before casting?
	refPolicy := *object.(*gateway.ReferencePolicy)
	r.Log.Info("event for ReferencePolicy", "object", refPolicy)

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

func (r *HTTPRouteReconciler) getRoutesAffectedByReferencePolicy(refPolicy gateway.ReferencePolicy) []gateway.HTTPRoute {
	matches := []gateway.HTTPRoute{}

	// toSelectors := []fields.Selector{}
	// for _, to := range refPolicy.Spec.To {
	// 	// When empty, the Kubernetes core API group is inferred.
	// 	group := "core/v1"
	// 	if to.Group != "" {
	// 		group = string(to.Group)
	// 	}

	// 	toSelectors = append(toSelectors, groupKindToFieldSelector(schema.GroupKind{
	// 		Group: group,
	// 		Kind:  string(to.Kind),
	// 	}))
	// }

	routes := r.getReferencePolicyObjectsFrom(refPolicy)

	// TODO: match only routes with BackendRefs selectable by a
	// ReferencePolicyTo instead of appending all routes. This seems expensive,
	// so not sure if it would actually improve performance or not.
	matches = append(matches, routes...)

	// for _, route := range routes {
	// 	routeMatched := false

	// 	// TODO: should this use reflection to handle xRoute types? seems expensive
	// 	for _, rule := range route.Spec.Rules {
	// 		for range rule.BackendRefs {
	// 			// Check if backendRef.BackendObjectReference is selectable by
	// 			// the requirements in any refPolicy.Spec.To
	// 			for _, selector := range toSelectors {
	// 				if selector.Matches(fields.Set{
	//                  "kind": backendRef.BackendObjectReference.Kind
	//                  "metadata.name": backendRef.BackendObjectReference.Name
	// 				}) {
	// 					routeMatched = true
	// 					matches = append(matches, route)

	// 					// Exit toSelectors loop early if route has already been matched
	// 					if routeMatched {
	// 						break
	// 					}
	// 				}
	// 			}

	// 			// Exit BackendRefs loop early if route has already been matched
	// 			if routeMatched {
	// 				break
	// 			}
	// 		}

	// 		// Exit Rules loop early if route has already been matched
	// 		if routeMatched {
	// 			break
	// 		}
	// 	}
	// }

	return matches
}

func (r *HTTPRouteReconciler) getReferencePolicyObjectsFrom(refPolicy gateway.ReferencePolicy) []gateway.HTTPRoute {
	matches := []gateway.HTTPRoute{}

	for _, from := range refPolicy.Spec.From {
		// TODO: search by from.Group and from.Kind instead of assuming HTTPRoute
		routes, err := r.Client.GetHTTPRoutesInNamespace(context.TODO(), string(from.Namespace))
		if err != nil {
			// TODO: is there a better way to handle this error?
			return matches
		}

		matches = append(matches, routes...)
	}

	return matches
}

func groupKindToFieldSelector(gk schema.GroupKind) fields.Selector {
	return fields.SelectorFromSet(fields.Set{
		"kind": gk.Kind,
	})
}
