package gatewayclient

import (
	"context"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

//go:generate mockgen -source ./gatewayclient.go -destination ./mocks/gatewayclient.go -package mocks Client

// Client is an abstraction around interactions with Kubernetes APIs. In order
// to keep the error boundaries clear for our reconciliation code, care should
// be taken to wrap all returned errors in a K8sError type so that any
// Kubernetes API errors can be retried immediately.
type Client interface {
	// getters
	GetGatewayClassConfig(ctx context.Context, key types.NamespacedName) (*apigwv1alpha1.GatewayClassConfig, error)
	GetGatewayClass(ctx context.Context, key types.NamespacedName) (*gateway.GatewayClass, error)
	GetGateway(ctx context.Context, key types.NamespacedName) (*gateway.Gateway, error)
	GetSecret(ctx context.Context, key types.NamespacedName) (*core.Secret, error)
	GetService(ctx context.Context, key types.NamespacedName) (*core.Service, error)
	GetHTTPRoute(ctx context.Context, key types.NamespacedName) (*gateway.HTTPRoute, error)

	// finalizer helpers

	GatewayClassInUse(ctx context.Context, gc *gateway.GatewayClass) (bool, error)
	GatewayClassConfigInUse(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error)
	RemoveFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error)
	EnsureFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error)

	// relationships

	HasManagedDeployment(ctx context.Context, gw *gateway.Gateway) (bool, error)
	IsManagedRoute(ctx context.Context, namespace string, parents []gateway.ParentRef) (bool, error)
	GetConfigForGatewayClassName(ctx context.Context, name string) (apigwv1alpha1.GatewayClassConfig, bool, error)
	DeploymentForGateway(ctx context.Context, gw *gateway.Gateway) (*apps.Deployment, error)
	SetControllerOwnership(owner, object client.Object) error

	// general utilities

	PodWithLabels(ctx context.Context, labels map[string]string) (*core.Pod, error)

	// status updates

	UpdateStatus(ctx context.Context, obj client.Object) error

	// updates

	Update(ctx context.Context, obj client.Object) error

	// deployments

	CreateOrUpdateDeployment(ctx context.Context, deployment *apps.Deployment, mutators ...func() error) (bool, error)
	CreateOrUpdateService(ctx context.Context, service *core.Service, mutators ...func() error) (bool, error)
	DeleteService(ctx context.Context, service *core.Service) error
}

type gatewayClient struct {
	client.Client
	scheme         *runtime.Scheme
	controllerName string
}

func New(client client.Client, scheme *runtime.Scheme, controllerName string) Client {
	return &gatewayClient{
		Client:         client,
		scheme:         scheme,
		controllerName: controllerName,
	}
}

func (g *gatewayClient) GatewayClassConfigInUse(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error) {
	list := &gateway.GatewayClassList{}
	if err := g.List(ctx, list); err != nil {
		return false, NewK8sError(err)
	}
	for _, gc := range list.Items {
		paramaterRef := gc.Spec.ParametersRef
		if paramaterRef != nil &&
			paramaterRef.Group == apigwv1alpha1.Group &&
			paramaterRef.Kind == apigwv1alpha1.GatewayClassConfigKind &&
			paramaterRef.Name == gcc.Name {

			// no need to check on namespaces since we're cluster-scoped
			return true, nil
		}
	}
	return false, nil
}

func (g *gatewayClient) GatewayClassInUse(ctx context.Context, gc *gateway.GatewayClass) (bool, error) {
	list := &gateway.GatewayList{}
	if err := g.List(ctx, list); err != nil {
		return false, NewK8sError(err)
	}
	for _, g := range list.Items {
		if string(g.Spec.GatewayClassName) == gc.Name {
			return true, nil
		}
	}
	return false, nil
}

func (g *gatewayClient) PodWithLabels(ctx context.Context, labels map[string]string) (*core.Pod, error) {
	list := &core.PodList{}
	if err := g.List(ctx, list, client.MatchingLabels(labels)); err != nil {
		return nil, NewK8sError(err)
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

	return nil, nil
}

func (g *gatewayClient) DeploymentForGateway(ctx context.Context, gw *gateway.Gateway) (*apps.Deployment, error) {
	deployment := &apps.Deployment{}
	key := types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}
	if err := g.Client.Get(ctx, key, deployment); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return deployment, nil
}

func (g *gatewayClient) GetGatewayClassConfig(ctx context.Context, key types.NamespacedName) (*apigwv1alpha1.GatewayClassConfig, error) {
	gcc := &apigwv1alpha1.GatewayClassConfig{}
	if err := g.Client.Get(ctx, key, gcc); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return gcc, nil
}

func (g *gatewayClient) GetGatewayClass(ctx context.Context, key types.NamespacedName) (*gateway.GatewayClass, error) {
	gc := &gateway.GatewayClass{}
	if err := g.Client.Get(ctx, key, gc); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return gc, nil
}

func (g *gatewayClient) GetGateway(ctx context.Context, key types.NamespacedName) (*gateway.Gateway, error) {
	gw := &gateway.Gateway{}
	if err := g.Client.Get(ctx, key, gw); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return gw, nil
}

func (g *gatewayClient) GetService(ctx context.Context, key types.NamespacedName) (*core.Service, error) {
	svc := &core.Service{}
	if err := g.Client.Get(ctx, key, svc); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return svc, nil
}

func (g *gatewayClient) GetSecret(ctx context.Context, key types.NamespacedName) (*core.Secret, error) {
	secret := &core.Secret{}
	if err := g.Client.Get(ctx, key, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return secret, nil
}

func (g *gatewayClient) GetHTTPRoute(ctx context.Context, key types.NamespacedName) (*gateway.HTTPRoute, error) {
	route := &gateway.HTTPRoute{}
	if err := g.Client.Get(ctx, key, route); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return route, nil
}

func (g *gatewayClient) EnsureFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error) {
	finalizers := object.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return false, nil
		}
	}
	object.SetFinalizers(append(finalizers, finalizer))
	if err := g.Update(ctx, object); err != nil {
		return false, NewK8sError(err)
	}
	return true, nil
}

// RemoveFinalizer ensures that the given finalizer is removed from the passed object
// it returns a boolean saying whether or not a finalizer was removed, and any
// potential errors
func (g *gatewayClient) RemoveFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error) {
	finalizers := []string{}
	found := false
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			found = true
			continue
		}
		finalizers = append(finalizers, f)
	}
	if found {
		object.SetFinalizers(finalizers)
		if err := g.Update(ctx, object); err != nil {
			return false, NewK8sError(err)
		}
	}
	return found, nil
}

func (g *gatewayClient) UpdateStatus(ctx context.Context, obj client.Object) error {
	if err := g.Status().Update(ctx, obj); err != nil {
		return NewK8sError(err)
	}
	return nil
}

func (g *gatewayClient) Update(ctx context.Context, obj client.Object) error {
	if err := g.Client.Update(ctx, obj); err != nil {
		return NewK8sError(err)
	}
	return nil
}

func (g *gatewayClient) CreateOrUpdateDeployment(ctx context.Context, deployment *apps.Deployment, mutators ...func() error) (bool, error) {
	operation, err := controllerutil.CreateOrUpdate(ctx, g.Client, deployment, func() error {
		for _, mutate := range mutators {
			if err := mutate(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, NewK8sError(err)
	}
	if operation == controllerutil.OperationResultCreated {
		metrics.Registry.IncrCounter(metrics.K8sNewGatewayDeployments, 1)
	}
	return operation != controllerutil.OperationResultNone, nil
}

func (g *gatewayClient) CreateOrUpdateService(ctx context.Context, service *core.Service, mutators ...func() error) (bool, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, g.Client, service, func() error {
		for _, mutate := range mutators {
			if err := mutate(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, NewK8sError(err)
	}
	return op != controllerutil.OperationResultNone, nil
}

func (g *gatewayClient) DeleteService(ctx context.Context, service *core.Service) error {
	if err := g.Delete(ctx, service); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return NewK8sError(err)
	}
	return nil
}

func (g *gatewayClient) SetControllerOwnership(owner, object client.Object) error {
	if err := ctrl.SetControllerReference(owner, object, g.scheme); err != nil {
		return NewK8sError(err)
	}
	return nil
}

func (g *gatewayClient) GetConfigForGatewayClassName(ctx context.Context, name string) (apigwv1alpha1.GatewayClassConfig, bool, error) {
	class, err := g.GetGatewayClass(ctx, types.NamespacedName{Name: name})
	if err != nil {
		return apigwv1alpha1.GatewayClassConfig{}, false, NewK8sError(err)
	}
	if class == nil {
		// no class found
		return apigwv1alpha1.GatewayClassConfig{}, false, nil
	}
	if class.Spec.ControllerName != gateway.GatewayController(g.controllerName) {
		// we're not owned by this controller, so pretend we don't exist
		return apigwv1alpha1.GatewayClassConfig{}, false, nil
	}
	if ref := class.Spec.ParametersRef; ref != nil {
		// check that we're using a typed config
		if ref.Group != apigwv1alpha1.Group || ref.Kind != apigwv1alpha1.GatewayClassConfigKind {
			// pretend we have nothing since we don't support untyped configuration
			return apigwv1alpha1.GatewayClassConfig{}, false, nil
		}

		// ignore namespace since we're cluster-scoped
		found, err := g.GetGatewayClassConfig(ctx, types.NamespacedName{Name: ref.Name})
		if err != nil {
			return apigwv1alpha1.GatewayClassConfig{}, false, NewK8sError(err)
		}
		if found == nil {
			// we have an invalid configuration, and hence an invalid gatewayclass, so pretend
			// we don't exist
			return apigwv1alpha1.GatewayClassConfig{}, false, nil
		}
		return *found, true, nil
	}
	// we have a legal gatewayclass without a config, just return a default
	return apigwv1alpha1.GatewayClassConfig{}, true, nil
}

func (g *gatewayClient) IsManagedRoute(ctx context.Context, namespace string, parents []gateway.ParentRef) (bool, error) {
	// we look up a list of deployments that are managed by us, and try and check our references based on them.
	list := &apps.DeploymentList{}
	if err := g.Client.List(ctx, list, client.MatchingLabels(map[string]string{
		utils.ManagedLabel: "true",
	})); err != nil {
		return false, NewK8sError(err)
	}
	for _, ref := range parents {
		name, isGateway := utils.ReferencesGateway(namespace, ref)
		if isGateway {
			for _, deployment := range list.Items {
				deployment := &deployment
				if name == utils.GatewayByLabels(deployment) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (g *gatewayClient) HasManagedDeployment(ctx context.Context, gw *gateway.Gateway) (bool, error) {
	list := &apps.DeploymentList{}
	if err := g.Client.List(ctx, list, client.MatchingLabels(utils.LabelsForGateway(gw))); err != nil {
		return false, NewK8sError(err)
	}
	if len(list.Items) > 0 {
		return true, nil
	}
	return false, nil
}
