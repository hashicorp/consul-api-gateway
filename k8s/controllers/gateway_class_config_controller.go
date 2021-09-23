package controllers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

const (
	gatewayClassConfigFinalizer = "gateway-class-exists-finalizer.api-gateway.consul.hashicorp.com"
)

// GatewayClassConfigReconciler reconciles a GatewayClassConfig object
type GatewayClassConfigReconciler struct {
	Client gatewayclient.Client
	Log    hclog.Logger
}

//+kubebuilder:rbac:groups=api-gateway.consul.hashicorp.com,resources=gatewayclassconfigs,verbs=get
//+kubebuilder:rbac:groups=api-gateway.consul.hashicorp.com,resources=gatewayclassconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayClassConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.With("gatewayClassConfig", req.NamespacedName)

	gcc, err := r.Client.GetGatewayClassConfig(ctx, req.NamespacedName)
	if err != nil {
		logger.Error("failed to get GatewayClassConfig", "error", err)
		return ctrl.Result{}, err
	}

	if gcc == nil {
		// we've been deleted, no-op
		return ctrl.Result{}, nil
	}

	if !gcc.ObjectMeta.DeletionTimestamp.IsZero() {
		// we have a deletion, ensure we're not in use
		used, err := r.Client.GatewayClassConfigInUse(ctx, gcc)
		if err != nil {
			logger.Error("failed to check if the gateway class config is still in use", "error", err)
			return ctrl.Result{}, err
		}
		if used {
			return ctrl.Result{}, fmt.Errorf("gateway class config '%s' is still in use", gcc.Name)
		}
		if _, err := r.Client.RemoveFinalizer(ctx, gcc, gatewayClassConfigFinalizer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// we're creating or updating
	if _, err := r.Client.EnsureFinalizer(ctx, gcc, gatewayClassConfigFinalizer); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassConfigReconciler) SetupWithManager(mgr ctrl.Manager, scheme *runtime.Scheme) error {
	groupVersion := schema.GroupVersion{Group: apigwv1alpha1.Group, Version: "v1alpha1"}
	scheme.AddKnownTypes(groupVersion, &apigwv1alpha1.GatewayClassConfig{}, &apigwv1alpha1.GatewayClassConfigList{})
	metav1.AddToGroupVersion(scheme, groupVersion)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apigwv1alpha1.GatewayClassConfig{}).
		Complete(r)
}
