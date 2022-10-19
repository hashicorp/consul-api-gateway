package reconciler

import (
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

const (
	defaultListenerName = "default"

	gatewayMetaExternalSource = "external-source"
	gatewayMetaName           = "consul-api-gateway/k8s/Gateway.Name"
	gatewayMetaNamespace      = "consul-api-gateway/k8s/Gateway.Namespace"
)

var (
	_ store.Gateway = (*K8sGateway)(nil)
)

type K8sGateway struct {
	*gwv1beta1.Gateway
	GatewayState *state.GatewayState

	Config apigwv1alpha1.GatewayClassConfig
}

// newK8sGateway
func newK8sGateway(config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway, gatewayState *state.GatewayState) *K8sGateway {
	return &K8sGateway{
		Gateway:      gateway,
		GatewayState: gatewayState,
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
	var listeners []core.ResolvedListener
	for i, listener := range g.Gateway.Spec.Listeners {
		state := g.GatewayState.Listeners[i]
		if state.Valid() {
			listeners = append(listeners, g.resolveListener(state, listener))
		}
	}

	rgw := core.ResolvedGateway{
		ID: g.ID(),
		Meta: map[string]string{
			gatewayMetaExternalSource: "consul-api-gateway",
			gatewayMetaName:           g.Gateway.Name,
			gatewayMetaNamespace:      g.Gateway.Namespace,
		},
		Listeners: listeners,
	}

	if g.Config.Spec.ConnectionManagement.MaxConnections != nil {
		maxConns := uint32(*g.Config.Spec.ConnectionManagement.MaxConnections)
		rgw.MaxConnections = &maxConns
	}

	return rgw
}

func (g *K8sGateway) resolveListener(state *state.ListenerState, listener gwv1beta1.Listener) core.ResolvedListener {
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

func (g *K8sGateway) CanFetchSecrets(secrets []string) (bool, error) {
	certificates := make(map[string]struct{})
	for _, listener := range g.GatewayState.Listeners {
		for _, cert := range listener.TLS.Certificates {
			certificates[cert] = struct{}{}
		}
	}
	for _, secret := range secrets {
		if _, found := certificates[secret]; !found {
			return false, nil
		}
	}
	return true, nil
}

func listenerHostname(listener gwv1beta1.Listener) string {
	if listener.Hostname != nil {
		return string(*listener.Hostname)
	}
	return ""
}

func listenerName(listener gwv1beta1.Listener) string {
	if listener.Name != "" {
		return string(listener.Name)
	}
	return defaultListenerName
}
