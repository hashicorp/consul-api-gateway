// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gatewayclient

import (
	"context"
	"errors"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

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
	GetGatewayClass(ctx context.Context, key types.NamespacedName) (*gwv1beta1.GatewayClass, error)
	GetGateway(ctx context.Context, key types.NamespacedName) (*gwv1beta1.Gateway, error)
	GetGatewaysInNamespace(ctx context.Context, ns string) ([]gwv1beta1.Gateway, error)
	GetSecret(ctx context.Context, key types.NamespacedName) (*core.Secret, error)
	GetService(ctx context.Context, key types.NamespacedName) (*core.Service, error)
	GetHTTPRoute(ctx context.Context, key types.NamespacedName) (*gwv1alpha2.HTTPRoute, error)
	GetHTTPRoutes(ctx context.Context) ([]gwv1alpha2.HTTPRoute, error)
	GetHTTPRoutesInNamespace(ctx context.Context, ns string) ([]gwv1alpha2.HTTPRoute, error)
	GetTCPRoute(ctx context.Context, key types.NamespacedName) (*gwv1alpha2.TCPRoute, error)
	GetTCPRoutes(ctx context.Context) ([]gwv1alpha2.TCPRoute, error)
	GetTCPRoutesInNamespace(ctx context.Context, ns string) ([]gwv1alpha2.TCPRoute, error)
	GetMeshService(ctx context.Context, key types.NamespacedName) (*apigwv1alpha1.MeshService, error)
	GetNamespace(ctx context.Context, key types.NamespacedName) (*core.Namespace, error)
	GetDeployment(ctx context.Context, key types.NamespacedName) (*apps.Deployment, error)

	// finalizer helpers

	GatewayClassInUse(ctx context.Context, gc *gwv1beta1.GatewayClass) (bool, error)
	GatewayClassConfigInUse(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error)
	GatewayClassesUsingConfig(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (*gwv1beta1.GatewayClassList, error)
	RemoveFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error)
	EnsureFinalizer(ctx context.Context, object client.Object, finalizer string) (bool, error)

	// relationships

	HasManagedDeployment(ctx context.Context, gw *gwv1beta1.Gateway) (bool, error)
	IsManagedRoute(ctx context.Context, namespace string, parents []gwv1alpha2.ParentReference) (bool, error)
	GetConfigForGatewayClassName(ctx context.Context, name string) (apigwv1alpha1.GatewayClassConfig, bool, error)
	DeploymentForGateway(ctx context.Context, gw *gwv1beta1.Gateway) (*apps.Deployment, error)
	SetControllerOwnership(owner, object client.Object) error

	// general utilities

	PodsWithLabels(ctx context.Context, labels map[string]string) ([]core.Pod, error)

	// status updates

	UpdateStatus(ctx context.Context, obj client.Object) error

	// updates

	Update(ctx context.Context, obj client.Object) error

	// deployments

	CreateOrUpdateDeployment(ctx context.Context, deployment *apps.Deployment, mutators ...func() error) (bool, error)
	CreateOrUpdateSecret(ctx context.Context, secret *core.Secret, mutators ...func() error) (bool, error)
	CreateOrUpdateService(ctx context.Context, service *core.Service, mutators ...func() error) (bool, error)
	DeleteService(ctx context.Context, service *core.Service) error
	EnsureExists(ctx context.Context, obj client.Object, mutators ...func() error) (bool, error)
	EnsureServiceAccount(ctx context.Context, owner *gwv1beta1.Gateway, serviceAccount *core.ServiceAccount) error

	// referencepolicy
	GetReferenceGrantsInNamespace(ctx context.Context, namespace string) ([]gwv1alpha2.ReferenceGrant, error)
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

// gatewayClassUsesConfig determines whether a given GatewayClass references a
// given GatewayClassConfig. Since these resources are scoped to the cluster,
// namespace is not considered.
func gatewayClassUsesConfig(gc gwv1beta1.GatewayClass, gcc *apigwv1alpha1.GatewayClassConfig) bool {
	paramaterRef := gc.Spec.ParametersRef
	return paramaterRef != nil &&
		paramaterRef.Group == apigwv1alpha1.Group &&
		paramaterRef.Kind == apigwv1alpha1.GatewayClassConfigKind &&
		paramaterRef.Name == gcc.Name
}

// GatewayClassConfigInUse determines whether any GatewayClass in the cluster
// references the provided GatewayClassConfig.
func (g *gatewayClient) GatewayClassConfigInUse(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (bool, error) {
	list := &gwv1beta1.GatewayClassList{}
	if err := g.List(ctx, list); err != nil {
		return false, NewK8sError(err)
	}

	for _, gc := range list.Items {
		if gatewayClassUsesConfig(gc, gcc) {
			return true, nil
		}
	}

	return false, nil
}

// GatewayClassesUsingConfig returns the list of all GatewayClasses in the
// cluster that reference the provided GatewayClassConfig.
func (g *gatewayClient) GatewayClassesUsingConfig(ctx context.Context, gcc *apigwv1alpha1.GatewayClassConfig) (*gwv1beta1.GatewayClassList, error) {
	list, filteredList := &gwv1beta1.GatewayClassList{}, &gwv1beta1.GatewayClassList{}
	if err := g.List(ctx, list); err != nil {
		return nil, NewK8sError(err)
	}

	for _, gc := range list.Items {
		if gatewayClassUsesConfig(gc, gcc) {
			filteredList.Items = append(filteredList.Items, gc)
		}
	}

	return filteredList, nil
}

// GatewayClassInUse determines whether any Gateway in the cluster
// references the provided GatewayClass.
func (g *gatewayClient) GatewayClassInUse(ctx context.Context, gc *gwv1beta1.GatewayClass) (bool, error) {
	list := &gwv1beta1.GatewayList{}
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

func (g *gatewayClient) PodsWithLabels(ctx context.Context, labels map[string]string) ([]core.Pod, error) {
	list := &core.PodList{}
	if err := g.List(ctx, list, client.MatchingLabels(labels)); err != nil {
		return nil, NewK8sError(err)
	}

	items := []core.Pod{}

	// return all pods that don't have a deletion timestamp
	for _, pod := range list.Items {
		if pod.DeletionTimestamp.IsZero() {
			items = append(items, pod)
		}
	}

	return items, nil
}

func (g *gatewayClient) DeploymentForGateway(ctx context.Context, gw *gwv1beta1.Gateway) (*apps.Deployment, error) {
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

func (g *gatewayClient) GetGatewayClass(ctx context.Context, key types.NamespacedName) (*gwv1beta1.GatewayClass, error) {
	gc := &gwv1beta1.GatewayClass{}
	if err := g.Client.Get(ctx, key, gc); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return gc, nil
}

func (g *gatewayClient) GetGateway(ctx context.Context, key types.NamespacedName) (*gwv1beta1.Gateway, error) {
	gw := &gwv1beta1.Gateway{}
	if err := g.Client.Get(ctx, key, gw); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return gw, nil
}

func (g *gatewayClient) GetGatewaysInNamespace(ctx context.Context, ns string) ([]gwv1beta1.Gateway, error) {
	gwList := &gwv1beta1.GatewayList{}
	if err := g.Client.List(ctx, gwList, client.InNamespace(ns)); err != nil {
		return []gwv1beta1.Gateway{}, NewK8sError(err)
	}
	return gwList.Items, nil
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

func (g *gatewayClient) GetDeployment(ctx context.Context, key types.NamespacedName) (*apps.Deployment, error) {
	depl := &apps.Deployment{}
	if err := g.Client.Get(ctx, key, depl); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return depl, nil
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

func (g *gatewayClient) GetHTTPRoute(ctx context.Context, key types.NamespacedName) (*gwv1alpha2.HTTPRoute, error) {
	route := &gwv1alpha2.HTTPRoute{}
	if err := g.Client.Get(ctx, key, route); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return route, nil
}

func (g *gatewayClient) GetHTTPRoutes(ctx context.Context) ([]gwv1alpha2.HTTPRoute, error) {
	routeList := &gwv1alpha2.HTTPRouteList{}
	if err := g.Client.List(ctx, routeList); err != nil {
		return []gwv1alpha2.HTTPRoute{}, NewK8sError(err)
	}
	return routeList.Items, nil
}

// TODO: Make this generic over Group and Kind, returning []client.Object
func (g *gatewayClient) GetHTTPRoutesInNamespace(ctx context.Context, ns string) ([]gwv1alpha2.HTTPRoute, error) {
	routeList := &gwv1alpha2.HTTPRouteList{}
	if err := g.Client.List(ctx, routeList, client.InNamespace(ns)); err != nil {
		return []gwv1alpha2.HTTPRoute{}, NewK8sError(err)
	}
	return routeList.Items, nil
}

func (g *gatewayClient) GetTCPRoute(ctx context.Context, key types.NamespacedName) (*gwv1alpha2.TCPRoute, error) {
	route := &gwv1alpha2.TCPRoute{}
	if err := g.Client.Get(ctx, key, route); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return route, nil
}

func (g *gatewayClient) GetTCPRoutes(ctx context.Context) ([]gwv1alpha2.TCPRoute, error) {
	routeList := &gwv1alpha2.TCPRouteList{}
	if err := g.Client.List(ctx, routeList); err != nil {
		return []gwv1alpha2.TCPRoute{}, NewK8sError(err)
	}
	return routeList.Items, nil
}

func (g *gatewayClient) GetTCPRoutesInNamespace(ctx context.Context, ns string) ([]gwv1alpha2.TCPRoute, error) {
	routeList := &gwv1alpha2.TCPRouteList{}
	if err := g.Client.List(ctx, routeList, client.InNamespace(ns)); err != nil {
		return []gwv1alpha2.TCPRoute{}, NewK8sError(err)
	}
	return routeList.Items, nil
}

func (g *gatewayClient) GetMeshService(ctx context.Context, key types.NamespacedName) (*apigwv1alpha1.MeshService, error) {
	service := &apigwv1alpha1.MeshService{}
	if err := g.Client.Get(ctx, key, service); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return service, nil
}

func (g *gatewayClient) GetNamespace(ctx context.Context, key types.NamespacedName) (*core.Namespace, error) {
	namespace := &core.Namespace{}
	if err := g.Client.Get(ctx, key, namespace); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, NewK8sError(err)
	}
	return namespace, nil
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

func multiMutatorFn(mutators []func() error) func() error {
	return func() error {
		for _, mutate := range mutators {
			if err := mutate(); err != nil {
				return err
			}
		}
		return nil
	}
}

func (g *gatewayClient) CreateOrUpdateDeployment(ctx context.Context, deployment *apps.Deployment, mutators ...func() error) (bool, error) {
	operation, err := controllerutil.CreateOrUpdate(ctx, g.Client, deployment, multiMutatorFn(mutators))
	if err != nil {
		return false, NewK8sError(err)
	}
	if operation == controllerutil.OperationResultCreated {
		metrics.Registry.IncrCounter(metrics.K8sNewGatewayDeployments, 1)
	}
	return operation != controllerutil.OperationResultNone, nil
}

func (g *gatewayClient) CreateOrUpdateSecret(ctx context.Context, secret *core.Secret, mutators ...func() error) (bool, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, g.Client, secret, multiMutatorFn(mutators))
	if err != nil {
		return false, NewK8sError(err)
	}
	return op != controllerutil.OperationResultNone, nil
}

func (g *gatewayClient) CreateOrUpdateService(ctx context.Context, service *core.Service, mutators ...func() error) (bool, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, g.Client, service, multiMutatorFn(mutators))
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

func (g *gatewayClient) EnsureExists(ctx context.Context, obj client.Object, mutators ...func() error) (bool, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, g.Client, obj, multiMutatorFn(mutators))
	if err != nil {
		return false, NewK8sError(err)
	}
	return op != controllerutil.OperationResultNone, nil
}

func (g *gatewayClient) EnsureServiceAccount(ctx context.Context, owner *gwv1beta1.Gateway, serviceAccount *core.ServiceAccount) error {
	created := &core.ServiceAccount{}
	key := types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}
	if err := g.Client.Get(ctx, key, created); err != nil {
		if k8serrors.IsNotFound(err) {
			if err := g.SetControllerOwnership(owner, serviceAccount); err != nil {
				return err
			}
			if err := g.Client.Create(ctx, serviceAccount); err != nil {
				return NewK8sError(err)
			}
			return nil
		}
		return NewK8sError(err)
	}
	for _, ref := range created.GetOwnerReferences() {
		if ref.UID == owner.GetUID() && ref.Name == owner.GetName() {
			// we found proper ownership
			return nil
		}
	}
	// we found the object, but we're not the owner of it, return an error
	return errors.New("service account not owned by the gateway")
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
	if class.Spec.ControllerName != gwv1beta1.GatewayController(g.controllerName) {
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

// TODO: this likely needs to support gwv1alpha2.ParentReference too
func (g *gatewayClient) IsManagedRoute(ctx context.Context, namespace string, parents []gwv1alpha2.ParentReference) (bool, error) {
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

func (g *gatewayClient) HasManagedDeployment(ctx context.Context, gw *gwv1beta1.Gateway) (bool, error) {
	list := &apps.DeploymentList{}
	if err := g.Client.List(ctx, list, client.MatchingLabels(utils.LabelsForGateway(gw))); err != nil {
		return false, NewK8sError(err)
	}
	return len(list.Items) > 0, nil
}

func (g *gatewayClient) GetReferenceGrantsInNamespace(ctx context.Context, namespace string) ([]gwv1alpha2.ReferenceGrant, error) {
	refGrantList := &gwv1alpha2.ReferenceGrantList{}
	if err := g.Client.List(ctx, refGrantList, client.InNamespace(namespace)); err != nil {
		return nil, NewK8sError(err)
	}
	refGrants := refGrantList.Items

	// Lookup ReferencePolicies here too for backwards compatibility, create
	// ReferenceGrants from them, and add them to list
	refPolicyList := &gwv1alpha2.ReferencePolicyList{}
	if err := g.Client.List(ctx, refPolicyList, client.InNamespace(namespace)); err != nil {
		return nil, NewK8sError(err)
	}
	for _, refPolicy := range refPolicyList.Items {
		refGrants = append(refGrants, gwv1alpha2.ReferenceGrant{Spec: refPolicy.Spec})
	}

	return refGrants, nil
}
