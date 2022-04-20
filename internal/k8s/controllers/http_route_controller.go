package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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
			// TODO: how does this handle deletes?
			&source.Kind{Type: &gateway.ReferencePolicy{}},
			handler.EnqueueRequestsFromMapFunc(r.referencePolicyToRouteRequests),
		).
		Complete(gatewayclient.NewRequeueingMiddleware(r.Log, r))
}

func getReferencePolicyObjectsFrom(refPolicy gateway.ReferencePolicy) []client.Object {
	matches := []client.Object{}

	for _, from := range refPolicy.Spec.From {
		matchLabels := map[string]string{
			"kubernetes.io/metadata.kind":      string(from.Kind),
			"kubernetes.io/metadata.namespace": string(from.Namespace),
		}

		if from.Group != "" {
			matchLabels["kubernetes.io/metadata.group"] = string(from.Group)
		} else {
			// When empty, the Kubernetes core API group is inferred.
			matchLabels["kubernetes.io/metadata.group"] = "core/v1"
		}

		selector := metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "",
				Operator: "In",
				Values:   []string{},
			}},
			MatchLabels: matchLabels,
		}

		matches = append(matches, []client.Object{}...)
	}

	return matches
}

func getReferencePolicyObjectsTo(refPolicy gateway.ReferencePolicy) []client.Object {
	matches := []client.Object{}

	for _, to := range refPolicy.Spec.To {
		selector := labels.NewSelector()

		kindReq, _ := labels.NewRequirement(
			"kubernetes.io/metadata.kind",
			selection.In,
			[]string{string(to.Kind)},
		)

		namespaceReq, _ := labels.NewRequirement(
			"kubernetes.io/metadata.namesapce",
			selection.In,
			[]string{refPolicy.Namespace},
		)

		selector = selector.Add(*kindReq, *namespaceReq)

		var groupReq *labels.Requirement
		if to.Group != "" {
			groupReq, _ = labels.NewRequirement(
				"kubernetes.io/metadata.group",
				selection.In,
				[]string{string(to.Group)},
			)
		} else {
			// When empty, the Kubernetes core API group is inferred.
			groupReq, _ = labels.NewRequirement(
				"kubernetes.io/metadata.group",
				selection.In,
				[]string{"core/v1"},
			)
		}

		selector = selector.Add(*groupReq)

		if to.Name != nil {
			nameReq, _ := labels.NewRequirement(
				"kubernetes.io/metadata.name",
				selection.In,
				[]string{string(*to.Name)},
			)
			selector = selector.Add(*nameReq)
		}

		// TODO: use selector
		matches = append(matches, []client.Object{}...)
	}

	return matches
}

func GetRoutesAffectedByReferencePolicy(refPolicy gateway.ReferencePolicy) []gateway.HTTPRoute {
	matches := []gateway.HTTPRoute{}
	objectsTo := getReferencePolicyObjectsTo(refPolicy)

	// FIXME: Only checking Routes selected by GetReferencePolicyObjectsFrom isn't
	// enough, need to reconcile Routes which may have been allowed before but no
	// longer are permitted
	//
	// GET Routes WHERE BackendRef is selected by GetReferencePolicyObjectsTo
	for _, route := range getReferencePolicyObjectsFrom(refPolicy) {
		// TODO: should this use reflection to handle xRoute types? seems expensive
		for _, rules := range route.Spec.Rules {
			for _, backendRef := range rules.BackendRefs {
				for _, from := range objectsTo {
					if backendRef.DeepEqual(from) {
						matches = append(matches, backendRef)
					}
				}
			}
		}
	}

	return matches
}

func (r *HTTPRouteReconciler) referencePolicyToRouteRequests(object client.Object) []reconcile.Request {
	r.Log.Info("event for ReferencePolicy", "object", object)

	refPolicy := gateway.ReferencePolicy{}
	routes := GetRoutesAffectedByReferencePolicy(refPolicy)
	requests := []reconcile.Request{}

	for _, route := range routes {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		})
	}

	if len(requests) > 0 {
		return requests
	}

	return nil
}
