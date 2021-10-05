package reconciler

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/state"
)

//go:generate mockgen -source ./manager.go -destination ./mocks/manager.go -package mocks ReconcileManager

type ReconcileManager interface {
	UpsertGatewayClass(ctx context.Context, gc *gw.GatewayClass, validParameters bool) error
	UpsertGateway(ctx context.Context, g *gw.Gateway) error
	UpsertRoute(ctx context.Context, r Route) error
	DeleteGatewayClass(ctx context.Context, name string) error
	DeleteGateway(ctx context.Context, name types.NamespacedName) error
	DeleteRoute(ctx context.Context, name types.NamespacedName) error
}

// GatewayReconcileManager manages a GatewayReconciler for each Gateway and is the interface by which Consul operations
// should be invoked in a kubernetes controller.
type GatewayReconcileManager struct {
	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
	consul         *api.Client

	state          *state.State
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
	State          *state.State
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
		state:          config.State,
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

	return m.state.AddGateway(ctx, NewK8sGateway(g, K8sGatewayConfig{
		ConsulNamespace: consulNamespace,
		Logger:          m.logger,
		Client:          m.client,
	}))
}

func (m *GatewayReconcileManager) UpsertRoute(ctx context.Context, r Route) error {
	return m.state.AddRoute(ctx, NewK8sRoute(r, K8sRouteConfig{
		ControllerName: m.controllerName,
		Logger:         m.logger,
		Client:         m.client,
		Consul:         m.consul,
	}))
}

func (m *GatewayReconcileManager) DeleteGatewayClass(ctx context.Context, name string) error {
	m.gatewayClasses.Delete(name)
	return nil
}

func (m *GatewayReconcileManager) DeleteGateway(ctx context.Context, name types.NamespacedName) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	err := m.state.DeleteGateway(ctx, state.GatewayID{
		Service:         name.Name,
		ConsulNamespace: m.namespaceMap[name],
	})

	if err != nil {
		return err
	}

	delete(m.namespaceMap, name)

	return nil
}

func (m *GatewayReconcileManager) DeleteRoute(ctx context.Context, name types.NamespacedName) error {
	return m.state.DeleteRoute(ctx, name.String())
}
