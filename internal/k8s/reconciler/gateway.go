package reconciler

import (
	"context"
	"encoding/json"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

type K8sGateway struct {
	*state.GatewayState
	*gw.Gateway

	listeners []*K8sListener

	consulNamespace string
	logger          hclog.Logger
	client          gatewayclient.Client
	config          apigwv1alpha1.GatewayClassConfig
	deployer        *GatewayDeployer
}

var _ store.StatusTrackingGateway = &K8sGateway{}

// TODO: remove this
func (g *K8sGateway) SetState(state *state.GatewayState) {
	g.GatewayState = state
	for i, listener := range g.listeners {
		listener.ListenerState = state.Listeners[i]
	}
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

func (g *K8sGateway) Listeners() []store.Listener {
	listeners := []store.Listener{}

	for _, listener := range g.listeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

// Bind returns the name of the listeners to which a route bound
func (g *K8sGateway) Bind(ctx context.Context, route store.Route) []string {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return nil
	}

	return NewBinder(g.client, g.Gateway, g.GatewayState).Bind(ctx, k8sRoute)
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

	gatewayStatus := g.GetStatus(g.Gateway)
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
