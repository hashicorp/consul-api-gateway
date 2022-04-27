package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
	Gateway         *gw.Gateway
	Config          apigwv1alpha1.GatewayClassConfig
	State           *state.GatewayState
	ConsulNamespace string
}

func (f *Factory) NewGateway(config NewGatewayConfig) *K8sGateway {
	gatewayState := config.State
	if gatewayState == nil {
		gatewayState = state.InitialGatewayState(config.Gateway)
	}
	return f.initializeGateway(&K8sGateway{
		Gateway:         config.Gateway,
		GatewayState:    gatewayState,
		config:          config.Config,
		consulNamespace: config.ConsulNamespace,
	})
}

func (f *Factory) initializeGateway(gateway *K8sGateway) *K8sGateway {
	logger := f.logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)
	gateway.logger = logger
	gateway.client = f.client
	gateway.deployer = f.deployer

	// get rid of this
	listeners := []*K8sListener{}
	for index, listener := range gateway.Spec.Listeners {
		k8sListener := NewK8sListener(gateway.Gateway, listener, K8sListenerConfig{
			ConsulNamespace: gateway.consulNamespace,
			Logger:          logger,
			Client:          f.client,
		})
		k8sListener.status = &(gateway.GatewayState.Listeners[index].Status)
		listeners = append(listeners, k8sListener)
	}
	gateway.listeners = listeners

	return gateway
}

func (f *Factory) NewRoute(route Route, state *state.RouteState) *K8sRoute {
	return f.initializeRoute(&K8sRoute{
		Route:      route,
		RouteState: state,
	})
}

func (f *Factory) initializeRoute(route *K8sRoute) *K8sRoute {
	route.logger = f.logger.Named("route").With("name", route.GetName())
	route.client = f.client
	route.controllerName = f.controllerName

	// get rid of this when validators are added
	route.resolver = f.resolver

	return route
}
