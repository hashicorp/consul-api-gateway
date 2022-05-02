package reconciler

import (
	"context"
	"encoding/json"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

const (
	defaultListenerName = "default"
)

type K8sGateway struct {
	*gw.Gateway
	GatewayState *state.GatewayState

	deployer        *GatewayDeployer
	consulNamespace string
	logger          hclog.Logger
	client          gatewayclient.Client
	config          apigwv1alpha1.GatewayClassConfig
}

var _ store.StatusTrackingGateway = &K8sGateway{}

type K8sGatewayConfig struct {
	ConsulNamespace string
	ConsulCA        string
	SDSHost         string
	SDSPort         int
	Config          apigwv1alpha1.GatewayClassConfig
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func (g *K8sGateway) ID() core.GatewayID {
	return core.GatewayID{
		Service:         g.Gateway.Name,
		ConsulNamespace: g.consulNamespace,
	}
}

func (g *K8sGateway) Meta() map[string]string {
	return map[string]string{
		"external-source":                          "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":      g.Gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace": g.Gateway.Namespace,
	}
}

// Bind returns the name of the listeners to which a route bound
func (g *K8sGateway) Bind(ctx context.Context, route store.Route) []string {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return nil
	}

	return NewBinder(g.client, g.Gateway, g.GatewayState).Bind(ctx, k8sRoute)
}

func (g *K8sGateway) Remove(ctx context.Context, routeID string) error {
	for _, listener := range g.GatewayState.Listeners {
		delete(listener.Routes, routeID)
	}

	return nil
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
		ID:        g.ID(),
		Meta:      g.Meta(),
		Listeners: listeners,
	}
}

func (g *K8sGateway) CanFetchSecrets(ctx context.Context, secrets []string) (bool, error) {
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

func (g *K8sGateway) TrackSync(ctx context.Context, sync func() (bool, error)) error {
	// we've done all but synced our state, so ensure our deployments are up-to-date
	if err := g.deployer.Deploy(ctx, g.consulNamespace, g.config, g.Gateway); err != nil {
		return err
	}

	didSync, err := sync()
	if err != nil {
		g.GatewayState.Status.InSync.SyncError = err
	} else if didSync {
		// clear out any old synchronization error statuses
		g.GatewayState.Status.InSync = status.GatewayInSyncStatus{}
	}

	gatewayStatus := g.GatewayState.GetStatus(g.Gateway)
	if !status.GatewayStatusEqual(gatewayStatus, g.Gateway.Status) {
		g.Gateway.Status = gatewayStatus
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(gatewayStatus, "", "  ")
			if err == nil {
				g.logger.Trace("setting gateway status", "status", string(data))
			}
		}
		if err := g.client.UpdateStatus(ctx, g.Gateway); err != nil {
			// make sure we return an error immediately that's unwrapped
			return err
		}
	}
	return nil
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
