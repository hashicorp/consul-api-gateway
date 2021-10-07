package reconciler

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
)

//go:generate mockgen -source ./manager.go -destination ./mocks/manager.go -package mocks ReconcileManager

type ReconcileManager interface {
	UpsertGatewayClass(ctx context.Context, gc *gw.GatewayClass, validParameters bool) error
	UpsertGateway(ctx context.Context, g *gw.Gateway) error
	UpsertHTTPRoute(ctx context.Context, r Route) error
	UpsertTCPRoute(ctx context.Context, r Route) error
	UpsertTLSRoute(ctx context.Context, r Route) error
	DeleteGatewayClass(ctx context.Context, name string) error
	DeleteGateway(ctx context.Context, name types.NamespacedName) error
	DeleteHTTPRoute(ctx context.Context, name types.NamespacedName) error
	DeleteTCPRoute(ctx context.Context, name types.NamespacedName) error
	DeleteTLSRoute(ctx context.Context, name types.NamespacedName) error
}

// GatewayReconcileManager manages a GatewayReconciler for each Gateway and is the interface by which Consul operations
// should be invoked in a kubernetes controller.
type GatewayReconcileManager struct {
	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
	consul         *api.Client

	store          core.Store
	gatewayClasses *K8sGatewayClasses

	namespaceMap map[types.NamespacedName]string
	// guards the above map
	mutex sync.RWMutex
}

var _ ReconcileManager = &GatewayReconcileManager{}

type ManagerConfig struct {
	ControllerName string
	Client         gatewayclient.Client
	Consul         *api.Client
	Status         client.StatusWriter
	Store          core.Store
	Logger         hclog.Logger
}

func NewReconcileManager(config ManagerConfig) *GatewayReconcileManager {
	return &GatewayReconcileManager{
		controllerName: config.ControllerName,
		logger:         config.Logger,
		client:         config.Client,
		consul:         config.Consul,
		gatewayClasses: NewK8sGatewayClasses(config.Logger.Named("gatewayclasses"), config.Client),
		namespaceMap:   make(map[types.NamespacedName]string),
		store:          config.Store,
	}
}

func (m *GatewayReconcileManager) UpsertGatewayClass(ctx context.Context, gc *gw.GatewayClass, validParameters bool) error {
	return m.gatewayClasses.Upsert(ctx, gc, validParameters)
}

func (m *GatewayReconcileManager) UpsertGateway(ctx context.Context, g *gw.Gateway) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// TODO: do real namespace mapping
	consulNamespace := ""

	m.namespaceMap[utils.NamespacedName(g)] = consulNamespace

	gateway := NewK8sGateway(g, K8sGatewayConfig{
		ConsulNamespace: consulNamespace,
		Logger:          m.logger,
		Client:          m.client,
	})

	if err := gateway.Validate(ctx); err != nil {
		return err
	}

	return m.store.UpsertGateway(ctx, gateway)
}

func (m *GatewayReconcileManager) UpsertHTTPRoute(ctx context.Context, r Route) error {
	return m.upsertRoute(ctx, r)
}

func (m *GatewayReconcileManager) UpsertTCPRoute(ctx context.Context, r Route) error {
	return m.upsertRoute(ctx, r)
}

func (m *GatewayReconcileManager) UpsertTLSRoute(ctx context.Context, r Route) error {
	return m.upsertRoute(ctx, r)
}

func (m *GatewayReconcileManager) upsertRoute(ctx context.Context, r Route) error {
	route := NewK8sRoute(r, K8sRouteConfig{
		ControllerName: m.controllerName,
		Logger:         m.logger,
		Client:         m.client,
		Consul:         m.consul,
	})
	if err := route.Validate(ctx); err != nil {
		return err
	}
	return m.store.UpsertRoute(ctx, route)
}

func (m *GatewayReconcileManager) DeleteGatewayClass(ctx context.Context, name string) error {
	m.gatewayClasses.Delete(name)
	return nil
}

func (m *GatewayReconcileManager) DeleteGateway(ctx context.Context, name types.NamespacedName) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	err := m.store.DeleteGateway(ctx, core.GatewayID{
		Service:         name.Name,
		ConsulNamespace: m.namespaceMap[name],
	})

	if err != nil {
		return err
	}

	delete(m.namespaceMap, name)

	return nil
}

func (m *GatewayReconcileManager) DeleteHTTPRoute(ctx context.Context, name types.NamespacedName) error {
	return m.store.DeleteRoute(ctx, HTTPRouteID(name))
}

func (m *GatewayReconcileManager) DeleteTLSRoute(ctx context.Context, name types.NamespacedName) error {
	return m.store.DeleteRoute(ctx, TLSRouteID(name))
}

func (m *GatewayReconcileManager) DeleteTCPRoute(ctx context.Context, name types.NamespacedName) error {
	return m.store.DeleteRoute(ctx, TCPRouteID(name))
}
