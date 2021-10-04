package reconciler

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
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
}

var _ ReconcileManager = &GatewayReconcileManager{}

type ManagerConfig struct {
	ControllerName string
	Registry       *common.GatewaySecretRegistry
	Client         gatewayclient.Client
	Consul         *api.Client
	Status         client.StatusWriter
	Logger         hclog.Logger
}

func NewReconcileManager(config ManagerConfig) *GatewayReconcileManager {
	return &GatewayReconcileManager{
		controllerName: config.ControllerName,
		logger:         config.Logger,
		client:         config.Client,
		consul:         config.Consul,
		gatewayClasses: NewK8sGatewayClasses(config.Logger.Named("gatewayclasses"), config.Client),
		state: state.NewState(state.StateConfig{
			Registry: config.Registry,
			Consul:   config.Consul,
			Logger:   config.Logger.Named("state"),
		}),
	}
}

func (m *GatewayReconcileManager) UpsertGatewayClass(ctx context.Context, gc *gw.GatewayClass, validParameters bool) error {
	return m.gatewayClasses.Upsert(ctx, gc, validParameters)
}

func (m *GatewayReconcileManager) UpsertGateway(ctx context.Context, g *gw.Gateway) error {
	return m.state.AddGateway(ctx, NewK8sGateway(g, K8sGatewayConfig{
		ControllerName: m.controllerName,
		Logger:         m.logger,
		Client:         m.client,
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
	return m.state.DeleteGateway(ctx, name.String())
}

func (m *GatewayReconcileManager) DeleteRoute(ctx context.Context, name types.NamespacedName) error {
	return m.state.DeleteRoute(ctx, name.String())
}
