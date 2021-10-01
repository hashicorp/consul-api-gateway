package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

// var ErrPodNotCreated = errors.New("pod not yet created for gateway")

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	Client         gatewayclient.Client
	Log            hclog.Logger
	SDSServerHost  string
	SDSServerPort  int
	ControllerName string
	Tracker        reconciler.GatewayStatusTracker
	Manager        reconciler.ReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;get;create;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=list;get;create;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get

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
		r.Tracker.DeleteStatus(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	gc, err := r.Client.GatewayClassForGateway(ctx, gw)
	if err != nil {
		logger.Error("failed to get gateway class", "error", err)
		return ctrl.Result{}, err
	}

	if string(gc.Spec.Controller) != r.ControllerName {
		// we don't manage this gateway
		return ctrl.Result{}, nil
	}

	if err := r.Manager.UpsertGateway(ctx, gw); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	if err := r.ensureDeployment(ctx, gc, gw); err != nil {
		logger.Error("failed to ensure gateway deployment exists", "error", err)
		return ctrl.Result{}, err
	}

	// update status based on pod
	pod, err := r.Client.PodWithLabels(ctx, utils.LabelsForGateway(gw))
	if err != nil {
		if errors.Is(err, gatewayclient.ErrPodNotCreated) {
			// the pod hasn't been created yet, just no-op since we'll
			// eventually get the event from our Watch
			logger.Trace("gateway deployment pod not yet created")
			return ctrl.Result{}, nil
		}
		logger.Error("failed to get gateway deployment pod", "error", err)
		return ctrl.Result{}, err
	}
	conditions := utils.MapGatewayConditionsFromPod(pod)
	err = r.Tracker.UpdateStatus(req.NamespacedName, pod, conditions, func() error {
		if logger.IsTrace() {
			update, err := json.MarshalIndent(conditions, "", "  ")
			if err == nil {
				logger.Trace("gateway deployment pod status updated", "conditions", string(update))
			}
		}
		gw.Status.Conditions = conditions
		return r.Client.UpdateStatus(ctx, gw)
	})
	if err != nil {
		logger.Error("failed to update gateway status", "error", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) ensureDeployment(ctx context.Context, gc *gateway.GatewayClass, gw *gateway.Gateway) error {
	deployment, err := r.Client.DeploymentForGateway(ctx, gw)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment != nil {
		// we found a deployment, no-op
		return nil
	}

	// no deployment exists, create deployment for the gateway
	gcc, err := r.Client.GatewayClassConfigForGatewayClass(ctx, gc)
	if err != nil {
		return fmt.Errorf("failed to get gateway class config: %w", err)
	}

	if gcc == nil {
		gcc = &apigwv1alpha1.GatewayClassConfig{}
	}

	deployment = gcc.DeploymentFor(gw, apigwv1alpha1.SDSConfig{
		Host: r.SDSServerHost,
		Port: r.SDSServerPort,
	})
	// Create service for the gateway
	service := gcc.ServiceFor(gw)

	// Set Gateway instance as the owner and controller
	if err := r.Client.SetControllerOwnership(gw, deployment); err != nil {
		return fmt.Errorf("failed to initialize gateway deployment: %w", err)
	}
	err = r.Client.CreateDeployment(ctx, deployment)
	if err != nil {
		return fmt.Errorf("failed to create new gateway deployment: %w", err)
	}

	if service != nil {
		// Set Service instance as the owner and controller
		if err := r.Client.SetControllerOwnership(gw, service); err != nil {
			return fmt.Errorf("failed to initialize gateway service: %w", err)
		}
		err = r.Client.CreateService(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create gateway service: %w", err)
		}
	}

	metrics.Registry.IncrCounter(metrics.K8sNewGatewayDeployments, 1)
	return nil
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
		Complete(r)
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
