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

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	"github.com/hashicorp/consul-api-gateway/k8s/reconciler"
)

const (
	gatewayClassFinalizer = "gateway-exists-finalizer.networking.x-k8s.io"
)

// GatewayClassReconciler reconciles a GatewayClass object
type GatewayClassReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	ControllerName string
	Manager        *reconciler.GatewayReconcileManager
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("gatewayClass", req.NamespacedName)

	gc := &gateway.GatewayClass{}
	err := r.Get(ctx, req.NamespacedName, gc)
	// If the gateway object has been deleted (and we get an IsNotFound
	// error), we need to stop the associated deployment.
	if k8serrors.IsNotFound(err) {
		r.Manager.DeleteGatewayClass(req.NamespacedName.Name)
		return ctrl.Result{}, nil
	} else if err != nil {
		r.Log.Error(err, "failed to get GatewayClass", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}

	if string(gc.Spec.Controller) != r.ControllerName {
		// no-op if we don't manage this gateway class
		return ctrl.Result{}, nil
	}

	if gc.ObjectMeta.DeletionTimestamp.IsZero() {
		// we're creating or updating
		updated, err := ensureGatewayClassFinalizer(ctx, r.Client, gc)
		if err != nil {
			return ctrl.Result{}, err
		}
		if updated {
			// requeue for versioning
			return ctrl.Result{Requeue: true}, nil
		}
		// this validation is used for setting the gateway class accepted status
		valid, err := isValidGatewayClass(ctx, r.Client, gc)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Manager.UpsertGatewayClass(gc, valid)
	} else {
		used, err := gatewayClassInUse(ctx, r.Client, gc)
		if err != nil {
			r.Log.Error(err, "failed to check if the gateway class is still in use, requeuing", "error", err, "name", gc.Name)
			return ctrl.Result{}, err
		}
		if used {
			return ctrl.Result{}, fmt.Errorf("gateway class '%s' is still in use", gc.Name)
		}
		// remove finalizer
		if err := removeGatewayClassFinalizer(ctx, r.Client, gc); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func ensureGatewayClassFinalizer(ctx context.Context, client client.Client, gc *gateway.GatewayClass) (bool, error) {
	for _, finalizer := range gc.Finalizers {
		if finalizer == gatewayClassFinalizer {
			return false, nil
		}
	}
	updated := gc.DeepCopy()
	updated.Finalizers = append(updated.Finalizers, gatewayClassFinalizer)
	if err := client.Update(ctx, updated); err != nil {
		return false, fmt.Errorf("failed to add in-use finalizer: %w", err)
	}
	return true, nil
}

func removeGatewayClassFinalizer(ctx context.Context, client client.Client, gc *gateway.GatewayClass) error {
	finalizers := []string{}
	found := false
	for _, finalizer := range gc.Finalizers {
		if finalizer == gatewayClassFinalizer {
			found = true
			continue
		}
		finalizers = append(finalizers, finalizer)
	}
	if found {
		updated := gc.DeepCopy()
		updated.Finalizers = finalizers
		if err := client.Update(ctx, updated); err != nil {
			return fmt.Errorf("failed to remove in-use finalizer: %w", err)
		}
	}
	return nil
}

func isValidGatewayClass(ctx context.Context, client client.Client, gc *gateway.GatewayClass) (bool, error) {
	// only validate if we actually have a config reference
	if parametersRef := gc.Spec.ParametersRef; parametersRef != nil {
		// check that we're using a typed config
		if parametersRef.Group != apigwv1alpha1.Group || parametersRef.Kind != apigwv1alpha1.GatewayClassConfigKind {
			return false, nil
		}

		// try and retrieve the config
		found := &apigwv1alpha1.GatewayClassConfig{}
		name := types.NamespacedName{Name: parametersRef.Name}
		if parametersRef.Namespace != nil {
			name.Namespace = parametersRef.Name
		}
		err := client.Get(ctx, name, found)
		if k8serrors.IsNotFound(err) {
			// no config
			return false, nil
		} else if err != nil {
			return false, err
		}
	}

	return true, nil
}

func gatewayClassConfigForGatewayClass(ctx context.Context, client client.Client, gc *gateway.GatewayClass) (*apigwv1alpha1.GatewayClassConfig, error) {
	if parametersRef := gc.Spec.ParametersRef; parametersRef != nil {
		// try and retrieve the config
		found := &apigwv1alpha1.GatewayClassConfig{}
		name := types.NamespacedName{Name: parametersRef.Name}
		if parametersRef.Namespace != nil {
			name.Namespace = parametersRef.Name
		}
		err := client.Get(ctx, name, found)
		if err != nil {
			return nil, err
		}
		return found, nil
	}
	return nil, nil
}

func gatewayClassInUse(ctx context.Context, client client.Client, gc *gateway.GatewayClass) (bool, error) {
	list := &gateway.GatewayList{}
	if err := client.List(ctx, list); err != nil {
		return false, fmt.Errorf("failed to list gateways")
	}
	for _, g := range list.Items {
		if g.Spec.GatewayClassName == gc.Name {
			return true, nil
		}
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For()
		For(&gateway.GatewayClass{}).
		Complete(r)
}
