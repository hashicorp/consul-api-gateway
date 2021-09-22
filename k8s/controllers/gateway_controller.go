package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	"github.com/hashicorp/consul-api-gateway/k8s/reconciler"
	"github.com/hashicorp/consul-api-gateway/k8s/utils"
)

const (
	gatewayFinalizer = "gateway-finalizer.api-gateway.consul.hashicorp.com"
)

var ErrPodNotCreated = errors.New("pod not yet created for gateway")

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	SDSServerHost  string
	SDSServerPort  int
	ControllerName string
	Tracker        *utils.PodTracker
	Manager        *reconciler.GatewayReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("gateway", req.NamespacedName)

	gw := &gateway.Gateway{}
	err := r.Get(ctx, req.NamespacedName, gw)
	// If the gateway object has been deleted (and we get an IsNotFound
	// error), we need to clean up the cached resources. Owned objects
	// get deleted automatically
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.Manager.DeleteGateway(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get Gateway")
		return ctrl.Result{}, err
	}

	gc, err := gatewayClassForGateway(ctx, r.Client, gw)
	if err != nil {
		logger.Error(err, "failed to get GatewayClass")
		return ctrl.Result{}, err
	}

	if string(gc.Spec.Controller) != r.ControllerName {
		// we don't manage this gateway
		return ctrl.Result{}, err
	}

	if !gw.DeletionTimestamp.IsZero() {
		// we're getting deleted, clean up cached resources
		pod, err := podWithLabels(ctx, r.Client, utils.LabelsForNamedGateway(req.NamespacedName))
		if err != nil {
			if errors.Is(err, ErrPodNotCreated) {
				// the pod wasn't found, we'll just ignore this and remove the finalizer
				if _, err := utils.RemoveFinalizer(ctx, r.Client, gw, gatewayFinalizer); err != nil {
					logger.Error(err, "failed to remove gateway finalizer")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			// something bad happened, requeue
			logger.Error(err, "failed to get gateway pod")
			return ctrl.Result{}, err
		}
		// since we're doing this in a finalizr block, we should always get pod info
		// prior to the gateway actually getting removed
		r.Tracker.DeleteStatus(pod)

		// remove the finalizer so we can continue with deletion
		if _, err := utils.RemoveFinalizer(ctx, r.Client, gw, gatewayFinalizer); err != nil {
			logger.Error(err, "failed to remove gateway finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// first we ensure our finalizer exists
	updated, err := utils.EnsureFinalizer(ctx, r.Client, gw, gatewayFinalizer)
	if err != nil {
		return ctrl.Result{}, err
	}
	if updated {
		// requeue
		return ctrl.Result{Requeue: true}, nil
	}

	r.Manager.UpsertGateway(gw)

	// Check if the deployment already exists, if not create a new one
	if err := r.ensureDeployment(ctx, gc, gw); err != nil {
		logger.Error(err, "failed to ensure gateway deployment exists")
		return ctrl.Result{}, err
	}

	// update status based on pod
	pod, err := podWithLabels(ctx, r.Client, utils.LabelsForGateway(gw))
	if err != nil {
		if errors.Is(err, ErrPodNotCreated) {
			// the pod hasn't been created yet, just no-op since we'll
			// eventually get the event from our Watch
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get gateway pod")
		return ctrl.Result{}, err
	}
	conditions := utils.MapGatewayConditionsFromPod(pod)
	if r.Tracker.UpdateStatus(pod, conditions) {
		logger.Info("gateway deployment pod status updated", "conditions", conditions)
	}

	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) ensureDeployment(ctx context.Context, gc *gateway.GatewayClass, gw *gateway.Gateway) error {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, deployment)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to get deployment: %w", err)
		}

		// Create deployment for the gateway
		gcc, err := gatewayClassConfigForGatewayClass(ctx, r.Client, gc)
		if err != nil {
			return fmt.Errorf("failed to get gateway class config: %w", err)
		}

		deployment := gcc.DeploymentFor(gw, apigwv1alpha1.SDSConfig{
			Host: r.SDSServerHost,
			Port: r.SDSServerPort,
		})
		// Create service for the gateway
		service := gcc.ServiceFor(gw)

		// Set Gateway instance as the owner and controller
		if err := ctrl.SetControllerReference(gw, deployment, r.Scheme); err != nil {
			return fmt.Errorf("failed to initialize gateway deployment: %w", err)
		}
		err = r.Create(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to create new gateway deployment: %w", err)
		}

		if service != nil {
			// Set Service instance as the owner and controller
			if err := ctrl.SetControllerReference(gw, service, r.Scheme); err != nil {
				return fmt.Errorf("failed to initialize gateway service: %w", err)
			}
			err = r.Create(ctx, service)
			if err != nil {
				return fmt.Errorf("failed to create gateway service: %w", err)
			}
		}

		metrics.Registry.IncrCounter(metrics.K8sNewGatewayDeployments, 1)
	}

	return nil
}

func podWithLabels(ctx context.Context, k8sClient client.Client, labels map[string]string) (*corev1.Pod, error) {
	list := &corev1.PodList{}
	if err := k8sClient.List(ctx, list, client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	// if we only have a single item, return it
	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}

	// we could potentially have two pods based off of one in the process of deletion
	// return the first with a zero deletion timestamp
	for _, pod := range list.Items {
		if pod.DeletionTimestamp.IsZero() {
			return &pod, nil
		}
	}

	return nil, ErrPodNotCreated
}

func gatewayClassForGateway(ctx context.Context, client client.Client, gw *gateway.Gateway) (*gateway.GatewayClass, error) {
	gc := &gateway.GatewayClass{}
	if err := client.Get(ctx, types.NamespacedName{Name: gw.Spec.GatewayClassName}, gc); err != nil {
		return nil, fmt.Errorf("failed to get gateway")
	}
	return gc, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicate, err := predicate.LabelSelectorPredicate(
		*metav1.SetAsLabelSelector(map[string]string{
			utils.ManagedLabel: "true",
		}),
	)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway.Gateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
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
			}),
			builder.WithPredicates(predicate),
		).
		Complete(r)
}
