package reconciler

import (
	"github.com/hashicorp/go-hclog"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

type Factory struct {
	controllerName string
	logger         hclog.Logger
	client         gatewayclient.Client
	deployer       *GatewayDeployer

	resolver service.BackendResolver
}

type FactoryConfig struct {
	ControllerName string
	Logger         hclog.Logger
	Client         gatewayclient.Client
	Deployer       *GatewayDeployer

	// get rid of this when validators are added
	Resolver service.BackendResolver
}

func NewFactory(config FactoryConfig) *Factory {
	return &Factory{
		controllerName: config.ControllerName,
		logger:         config.Logger,
		client:         config.Client,
		deployer:       config.Deployer,
		resolver:       config.Resolver,
	}
}

type NewGatewayConfig struct {
	Gateway         *gwv1beta1.Gateway
	Config          apigwv1alpha1.GatewayClassConfig
	State           *state.GatewayState
	ConsulNamespace string
}

func (f *Factory) NewGateway(config NewGatewayConfig) *K8sGateway {
	gateway := NewK8sGateway(config.Gateway, K8sGatewayConfig{
		ConsulNamespace: config.ConsulNamespace,
		ConsulCA:        "",
		SDSHost:         "",
		SDSPort:         0,
		Config:          config.Config,
		Deployer:        f.deployer,
		Logger:          f.logger.Named("gateway").With("name", config.Gateway.Name, "namespace", config.Gateway.Namespace),
		Client:          f.client,
	})

	// TODO Consider moving into K8sGatewayConfig
	gateway.GatewayState = config.State
	if config.State == nil {
		gateway.GatewayState = state.InitialGatewayState(config.Gateway)
		gateway.GatewayState.ConsulNamespace = config.ConsulNamespace
	}

	return gateway
}

func (f *Factory) NewRoute(route Route) *K8sRoute {
	return f.NewRouteWithState(route, state.NewRouteState())
}

func (f *Factory) NewRouteWithState(route Route, state *state.RouteState) *K8sRoute {
	return NewK8sRoute(route, K8sRouteConfig{
		State:          state,
		Logger:         f.logger.Named("route").With("name", route.GetName()),
		Client:         f.client,
		ControllerName: f.controllerName,
		Resolver:       f.resolver,
	})
}
