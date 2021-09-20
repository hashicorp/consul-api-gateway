package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	"github.com/hashicorp/consul-api-gateway/k8s/reconciler"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	SDSServerHost  string
	SDSServerPort  int
	ControllerName string
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
	_ = r.Log.WithValues("gateway", req.NamespacedName)

	gw := &gateway.Gateway{}
	err := r.Get(ctx, req.NamespacedName, gw)
	// If the gateway object has been deleted (and we get an IsNotFound
	// error), we need to stop the associated deployment.
	if k8serrors.IsNotFound(err) {
		r.Manager.DeleteGateway(req.NamespacedName)
		// TODO stop deployment
		return ctrl.Result{}, nil
	} else if err != nil {
		r.Log.Error(err, "failed to get Gateway", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}

	r.Log.Info("retrieved", "name", gw.Name, "ns", gw.Namespace)
	r.Manager.UpsertGateway(gw)

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		// Create deployment for the gateway
		gc, err := gatewayClassForGateway(ctx, r.Client, gw)
		if err != nil {
			return ctrl.Result{}, err
		}
		if string(gc.Spec.Controller) == r.ControllerName {
			gcc, err := gatewayClassConfigForGatewayClass(ctx, r.Client, gc)
			if err != nil {
				return ctrl.Result{}, err
			}

			deployment := gcc.DeploymentFor(gw, apigwv1alpha1.SDSConfig{
				Host: r.SDSServerHost,
				Port: r.SDSServerPort,
			})
			// Create service for the gateway
			service := gcc.ServiceFor(gw)

			// Set Gateway instance as the owner and controller
			if err := ctrl.SetControllerReference(gw, deployment, r.Scheme); err != nil {
				log.Error(err, "Failed to initialize gateway deployment")
				return ctrl.Result{}, err
			}
			r.Log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			err = r.Create(ctx, deployment)
			if err != nil {
				log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
				return ctrl.Result{}, err
			}

			if service != nil {
				// Set Service instance as the owner and controller
				if err := ctrl.SetControllerReference(gw, service, r.Scheme); err != nil {
					log.Error(err, "Failed to initialize gateway service")
					return ctrl.Result{}, err
				}
				r.Log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
				err = r.Create(ctx, service)
				if err != nil {
					log.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
					return ctrl.Result{}, err
				}
			}

			metrics.Registry.IncrCounter(metrics.K8sNewGatewayDeployments, 1)

			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		}
		// we have an invalid controller class reference
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
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
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For()
		For(&gateway.Gateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
