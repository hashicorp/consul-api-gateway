package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/k8s/apis/v1alpha1"
	"github.com/hashicorp/consul-api-gateway/k8s/utils"
)

const (
	gatewayClassConfigFinalizer = "gateway-class-exists-finalizer.api-gateway.consul.hashicorp.com"
)

// GatewayClassConfigReconciler reconciles a GatewayClassConfig object
type GatewayClassConfigReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=api-gateway.consul.hashicorp.com,resources=gatewayclassconfigs,verbs=get
//+kubebuilder:rbac:groups=api-gateway.consul.hashicorp.com,resources=gatewayclassconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayClassConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("gatewayClassConfig", req.NamespacedName)

	gcc := &apigwv1alpha1.GatewayClassConfig{}
	err := r.Get(ctx, req.NamespacedName, gcc)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		r.Log.Error(err, "failed to get GatewayClassConfig", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}

	if gcc.ObjectMeta.DeletionTimestamp.IsZero() {
		// we're creating or updating
		if _, err := utils.EnsureFinalizer(ctx, r.Client, gcc, gatewayClassConfigFinalizer); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		used, err := gatewayClassConfigInUse(ctx, r.Client, gcc)
		if err != nil {
			r.Log.Error(err, "failed to check if the gateway class config is still in use, requeuing", "error", err, "name", gcc.Name)
			return ctrl.Result{}, err
		}
		if used {
			return ctrl.Result{}, fmt.Errorf("gateway class config '%s' is still in use", gcc.Name)
		}
		if _, err := utils.RemoveFinalizer(ctx, r.Client, gcc, gatewayClassConfigFinalizer); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func gatewayClassConfigInUse(ctx context.Context, client client.Client, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error) {
	list := &gateway.GatewayClassList{}
	if err := client.List(ctx, list); err != nil {
		return false, fmt.Errorf("failed to list gateways")
	}
	for _, g := range list.Items {
		paramaterRef := g.Spec.ParametersRef
		if paramaterRef != nil &&
			paramaterRef.Group == apigwv1alpha1.Group &&
			paramaterRef.Kind == apigwv1alpha1.GatewayClassConfigKind &&
			paramaterRef.Name == gcc.Name {
			namespace := ""
			if paramaterRef.Namespace != nil {
				namespace = string(*paramaterRef.Namespace)
			}
			if namespace == gcc.Namespace {
				return true, nil
			}
		}
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	groupVersion := schema.GroupVersion{Group: "api-gateway.consul.hashicorp.com", Version: "v1alpha1"}
	r.Scheme.AddKnownTypes(groupVersion, &apigwv1alpha1.GatewayClassConfig{}, &apigwv1alpha1.GatewayClassConfigList{})
	metav1.AddToGroupVersion(r.Scheme, groupVersion)

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For()
		For(&apigwv1alpha1.GatewayClassConfig{}).
		Complete(r)
}
