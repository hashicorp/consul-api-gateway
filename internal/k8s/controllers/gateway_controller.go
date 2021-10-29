package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/go-hclog"
)

// var ErrPodNotCreated = errors.New("pod not yet created for gateway")

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	Client         gatewayclient.Client
	Log            hclog.Logger
	ControllerName string
	Manager        reconciler.ReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;get;create;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=list;get;create;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.With("gateway", req.NamespacedName)

	gw, err := r.Client.GetGateway(ctx, req.NamespacedName)
	if err != nil {
		logger.Error("failed to get gateway", "error", err)
		return ctrl.Result{}, err
	}

	if gw == nil {
		// If the gateway object has been deleted (and we get an IsNotFound
		// error), we need to clean up the cached resources. Owned objects
		// get deleted automatically
		if err := r.Manager.DeleteGateway(ctx, req.NamespacedName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.Manager.UpsertGateway(ctx, gw); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicate, _ := predicate.LabelSelectorPredicate(
		*metav1.SetAsLabelSelector(map[string]string{
			utils.ManagedLabel: "true",
		}),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.Gateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			handler.EnqueueRequestsFromMapFunc(podToGatewayRequest),
			builder.WithPredicates(predicate),
		).
		Complete(gatewayclient.NewRequeueingMiddleware(r.Log, r))
}

func podToGatewayRequest(object client.Object) []reconcile.Request {
	gateway, managed := utils.IsManagedGateway(object.GetLabels())

	if managed {
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Name:      gateway,
				Namespace: object.GetNamespace(),
			}},
		}
	}
	return nil
}
