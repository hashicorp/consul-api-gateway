package gatewayclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/cenkalti/backoff"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

//go:generate mockgen -source ./gatewayclient.go -destination ./mocks/gatewayclient.go -package mocks Client

const (
	statusUpdateTimeout     = 10 * time.Second
	maxStatusUpdateAttempts = 5
)

var ErrPodNotCreated = errors.New("pod not yet created for gateway")

type Client interface {
	// getters
	GetGatewayClassConfig(ctx context.Context, key types.NamespacedName) (*apigwv1alpha1.GatewayClassConfig, error)
	GetGatewayClass(ctx context.Context, key types.NamespacedName) (*gateway.GatewayClass, error)
	GetGateway(ctx context.Context, key types.NamespacedName) (*gateway.Gateway, error)
	GetHTTPRoute(ctx context.Context, key types.NamespacedName) (*gateway.HTTPRoute, error)

	// finalizer helpers

	GatewayClassInUse(ctx context.Context, gc *gateway.GatewayClass) (bool, error)
	GatewayClassConfigInUse(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error)
	RemoveFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error)
	EnsureFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error)

	// relationships

	GatewayClassConfigForGatewayClass(ctx context.Context, gc *gateway.GatewayClass) (*apigwv1alpha1.GatewayClassConfig, error)
	GatewayClassForGateway(ctx context.Context, gw *gateway.Gateway) (*gateway.GatewayClass, error)
	DeploymentForGateway(ctx context.Context, gw *gateway.Gateway) (*apps.Deployment, error)
	SetControllerOwnership(owner, object client.Object) error

	// general utilities

	PodWithLabels(ctx context.Context, labels map[string]string) (*core.Pod, error)

	// validation

	IsValidGatewayClass(ctx context.Context, gc *gateway.GatewayClass) (bool, error)
	IsManagedRoute(ctx context.Context, spec gateway.CommonRouteSpec, routeNamespace, controllerName string) (bool, error)

	// status updates

	UpdateStatus(ctx context.Context, obj client.Object) error

	// deployments

	CreateDeployment(ctx context.Context, deployment *apps.Deployment) error
	CreateService(ctx context.Context, service *core.Service) error
}

type gatewayClient struct {
	client.Client
	scheme *runtime.Scheme
}

func New(client client.Client, scheme *runtime.Scheme) Client {
	return &gatewayClient{
		Client: client,
		scheme: scheme,
	}
}

func (g *gatewayClient) GatewayClassConfigInUse(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error) {
	list := &gateway.GatewayClassList{}
	if err := g.List(ctx, list); err != nil {
		return false, fmt.Errorf("failed to list gateway classes: %w", err)
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

func (g *gatewayClient) IsValidGatewayClass(ctx context.Context, gc *gateway.GatewayClass) (bool, error) {
	// only validate if we actually have a config reference
	if parametersRef := gc.Spec.ParametersRef; parametersRef != nil {
		// check that we're using a typed config
		if parametersRef.Group != apigwv1alpha1.Group || parametersRef.Kind != apigwv1alpha1.GatewayClassConfigKind {
			return false, nil
		}

		// try and retrieve the config
		found := &apigwv1alpha1.GatewayClassConfig{}
		name := types.NamespacedName{Name: parametersRef.Name}
		// ignore namespace since we're cluster-scoped
		err := g.Get(ctx, name, found)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// no config
				return false, nil
			}
			return false, err
		}
	}

	return true, nil
}

func (g *gatewayClient) GatewayClassConfigForGatewayClass(ctx context.Context, gc *gateway.GatewayClass) (*apigwv1alpha1.GatewayClassConfig, error) {
	if parametersRef := gc.Spec.ParametersRef; parametersRef != nil {
		if parametersRef.Group != apigwv1alpha1.Group || parametersRef.Kind != apigwv1alpha1.GatewayClassConfigKind {
			// don't try and retrieve if it's not the right type
			return nil, errors.New("wrong gateway class config type")
		}
		// try and retrieve the config
		found := &apigwv1alpha1.GatewayClassConfig{}
		// no namespaces since we're cluster-scoped
		name := types.NamespacedName{Name: parametersRef.Name}
		err := g.Get(ctx, name, found)
		if err != nil {
			return nil, err
		}
		return found, nil
	}
	return nil, nil
}

func (g *gatewayClient) GatewayClassInUse(ctx context.Context, gc *gateway.GatewayClass) (bool, error) {
	list := &gateway.GatewayList{}
	if err := g.List(ctx, list); err != nil {
		return false, fmt.Errorf("failed to list gateways: %w", err)
	}
	for _, g := range list.Items {
		if g.Spec.GatewayClassName == gc.Name {
			return true, nil
		}
	}
	return false, nil
}

func (g *gatewayClient) PodWithLabels(ctx context.Context, labels map[string]string) (*core.Pod, error) {
	list := &core.PodList{}
	if err := g.List(ctx, list, client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
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

func (g *gatewayClient) GatewayClassForGateway(ctx context.Context, gw *gateway.Gateway) (*gateway.GatewayClass, error) {
	gc := &gateway.GatewayClass{}
	if err := g.Get(ctx, types.NamespacedName{Name: gw.Spec.GatewayClassName}, gc); err != nil {
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	return gc, nil
}

func (g *gatewayClient) DeploymentForGateway(ctx context.Context, gw *gateway.Gateway) (*apps.Deployment, error) {
	deployment := &apps.Deployment{}
	key := types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}
	if err := g.Get(ctx, key, deployment); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return deployment, nil
}

func (g *gatewayClient) GetGatewayClassConfig(ctx context.Context, key types.NamespacedName) (*apigwv1alpha1.GatewayClassConfig, error) {
	gcc := &apigwv1alpha1.GatewayClassConfig{}
	if err := g.Get(ctx, key, gcc); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return gcc, nil
}

func (g *gatewayClient) GetGatewayClass(ctx context.Context, key types.NamespacedName) (*gateway.GatewayClass, error) {
	gc := &gateway.GatewayClass{}
	if err := g.Get(ctx, key, gc); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return gc, nil
}

func (g *gatewayClient) GetGateway(ctx context.Context, key types.NamespacedName) (*gateway.Gateway, error) {
	gw := &gateway.Gateway{}
	if err := g.Get(ctx, key, gw); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return gw, nil
}

func (g *gatewayClient) GetHTTPRoute(ctx context.Context, key types.NamespacedName) (*gateway.HTTPRoute, error) {
	route := &gateway.HTTPRoute{}
	if err := g.Get(ctx, key, route); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
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
		return false, fmt.Errorf("failed to add in-use finalizer: %w", err)
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
			return false, fmt.Errorf("failed to remove in-use finalizer: %w", err)
		}
	}
	return found, nil
}

func (g *gatewayClient) IsManagedRoute(ctx context.Context, spec gateway.CommonRouteSpec, routeNamespace, controllerName string) (bool, error) {
	for _, ref := range spec.ParentRefs {
		gw := &gateway.Gateway{}
		name := types.NamespacedName{Name: ref.Name}
		name.Namespace = routeNamespace
		if ref.Namespace != nil {
			name.Namespace = string(*ref.Namespace)
		}
		if err := g.Get(ctx, name, gw); err != nil {
			return false, fmt.Errorf("failed to get gateway: %w", err)
		}

		gc, err := g.GatewayClassForGateway(ctx, gw)
		if err != nil {
			return false, fmt.Errorf("failed to get gateway class: %w", err)
		}

		if string(gc.Spec.Controller) == controllerName {
			return true, err
		}
	}
	return false, nil
}

func (g *gatewayClient) UpdateStatus(ctx context.Context, obj client.Object) error {
	return backoff.Retry(func() error {
		return g.Status().Update(ctx, obj)
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(statusUpdateTimeout), maxStatusUpdateAttempts), ctx))
}

func (g *gatewayClient) CreateDeployment(ctx context.Context, deployment *apps.Deployment) error {
	return g.Create(ctx, deployment)
}

func (g *gatewayClient) CreateService(ctx context.Context, service *core.Service) error {
	return g.Create(ctx, service)
}

func (g *gatewayClient) SetControllerOwnership(owner, object client.Object) error {
	return ctrl.SetControllerReference(owner, object, g.scheme)
}
