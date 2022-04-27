package reconciler

import (
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

const (
	defaultListenerName = "default"
)

type K8sGateway struct {
	*gw.Gateway
	GatewayState *state.GatewayState
	Config       apigwv1alpha1.GatewayClassConfig
}

var _ store.Gateway = &K8sGateway{}

func NewGateway(config apigwv1alpha1.GatewayClassConfig, gateway *gw.Gateway, state *state.GatewayState) *K8sGateway {
	return &K8sGateway{
		Gateway:      gateway,
		GatewayState: state,
		Config:       config,
	}
}

func (g *K8sGateway) ID() core.GatewayID {
	return core.GatewayID{
		Service:         g.Gateway.Name,
		ConsulNamespace: g.GatewayState.ConsulNamespace,
	}
}

func (g *K8sGateway) Resolve() core.ResolvedGateway {
	listeners := []core.ResolvedListener{}
	for i, listener := range g.Gateway.Spec.Listeners {
		state := g.GatewayState.Listeners[i]
		if state.Valid() {
			listeners = append(listeners, g.resolveListener(state, listener))
		}
	}
	return core.ResolvedGateway{
		ID: g.ID(),
		Meta: map[string]string{
			"external-source":                          "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":      g.Gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace": g.Gateway.Namespace,
		},
		Listeners: listeners,
	}
}

func (g *K8sGateway) CanFetchSecrets(secrets []string) bool {
	certificates := make(map[string]struct{})
	for _, listener := range g.GatewayState.Listeners {
		for _, cert := range listener.TLS.Certificates {
			certificates[cert] = struct{}{}
		}
	}
	for _, secret := range secrets {
		if _, found := certificates[secret]; !found {
			return false
		}
	}
	return true
}

func (g *K8sGateway) resolveListener(state *state.ListenerState, listener gw.Listener) core.ResolvedListener {
	routes := []core.ResolvedRoute{}
	for _, route := range state.Routes {
		routes = append(routes, route)
	}
	protocol, _ := utils.ProtocolToConsul(state.Protocol)

	return core.ResolvedListener{
		Name:     listenerName(listener),
		Hostname: listenerHostname(listener),
		Port:     int(listener.Port),
		Protocol: protocol,
		TLS:      state.TLS,
		Routes:   routes,
	}
}

func listenerHostname(listener gw.Listener) string {
	if listener.Hostname != nil {
		return string(*listener.Hostname)
	}
	return ""
}

func listenerName(listener gw.Listener) string {
	if listener.Name != "" {
		return string(listener.Name)
	}
	return defaultListenerName
}
