package reconciler

import (
	"github.com/hashicorp/go-hclog"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

// TODO Remove Factory as it's no longer necessarygithub
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
	State           *state.GatewayState
	Config          apigwv1alpha1.GatewayClassConfig
	ConsulNamespace string
}

func (f *Factory) NewGateway(config NewGatewayConfig) *K8sGateway {
	gwState := config.State
	if gwState == nil {
		gwState = state.InitialGatewayState(config.Gateway)
		gwState.ConsulNamespace = config.ConsulNamespace
	}

	return newK8sGateway(config.Config, config.Gateway, gwState)
}

type NewRouteConfig struct {
	Route Route
	State *state.RouteState
}

func (f *Factory) NewRoute(config NewRouteConfig) *K8sRoute {
	routeState := config.State
	if routeState == nil {
		routeState = state.NewRouteState()
	}

	return newK8sRoute(config.Route, K8sRouteConfig{
		Logger:         f.logger.Named("route").With("name", config.Route.GetName()),
		Client:         f.client,
		ControllerName: f.controllerName,
		Resolver:       f.resolver,
		State:          routeState,
	})
}
