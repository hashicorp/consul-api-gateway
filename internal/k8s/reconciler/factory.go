package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/builder"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Factory struct {
	controllerName string
	consulCA       string
	sdsHost        string
	sdsPort        int
	logger         hclog.Logger
	client         gatewayclient.Client
}

type FactoryConfig struct {
	ControllerName string
	ConsulCA       string
	SDSHost        string
	SDSPort        int
	Logger         hclog.Logger
	Client         gatewayclient.Client
}

func NewFactory(config FactoryConfig) *Factory {
	return &Factory{
		controllerName: config.ControllerName,
		consulCA:       config.ConsulCA,
		sdsHost:        config.SDSHost,
		sdsPort:        config.SDSPort,
		logger:         config.Logger,
		client:         config.Client,
	}
}

type NewGatewayConfig struct {
	Gateway         *gw.Gateway
	ConsulNamespace string
	Config          apigwv1alpha1.GatewayClassConfig
}

func (f *Factory) NewGateway(config NewGatewayConfig) *K8sGateway {
	return f.initializeGateway(&K8sGateway{
		Gateway:         config.Gateway,
		GatewayState:    &state.GatewayState{},
		config:          config.Config,
		consulNamespace: config.ConsulNamespace,
	})
}

func (f *Factory) initializeGateway(gateway *K8sGateway) *K8sGateway {
	logger := f.logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)

	// TODO: get rid of this to make this marshalable
	listeners := []*K8sListener{}
	for _, listener := range gateway.Spec.Listeners {
		k8sListener := NewK8sListener(gateway, listener, K8sListenerConfig{
			Logger: logger,
			Client: f.client,
		})
		state := &state.ListenerState{}
		gateway.GatewayState.Listeners = append(gateway.GatewayState.Listeners, state)
		k8sListener.ListenerState = state
		listeners = append(listeners, k8sListener)
	}
	gateway.listeners = listeners

	deployment := builder.NewGatewayDeployment(gateway.Gateway)
	deployment.WithSDS(f.sdsHost, f.sdsPort)
	deployment.WithClassConfig(gateway.config)
	deployment.WithConsulCA(f.consulCA)
	deployment.WithConsulGatewayNamespace(gateway.consulNamespace)

	service := builder.NewGatewayService(gateway.Gateway)
	service.WithClassConfig(gateway.config)

	gateway.logger = logger
	gateway.deploymentBuilder = deployment
	gateway.serviceBuilder = service
	gateway.client = f.client

	return gateway
}

func (f *Factory) NewRoute(route Route) *K8sRoute {
	return f.initializeRoute(&K8sRoute{
		Route: route,
		RouteState: &state.RouteState{
			ResolutionErrors: service.NewResolutionErrors(),
			References:       make(service.RouteRuleReferenceMap),
			ParentStatuses:   make(status.RouteStatuses),
		},
	})
}

func (f *Factory) initializeRoute(route *K8sRoute) *K8sRoute {
	route.logger = f.logger.Named("route").With("name", route.GetName())
	route.client = f.client
	route.controllerName = f.controllerName

	return route
}

func (f *Factory) Marshal(v interface{}) ([]byte, error) {
	return nil, nil
}

func (f *Factory) Unmarshal(data []byte, v interface{}) error {
	return nil
}
