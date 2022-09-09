package reconciler

import (
	"context"
	"encoding/json"

	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/validator"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

const defaultListenerName = "default"

// TODO (nathancoleman) A lot of these fields - including validator, deployer, etc. -
//   will need to move out of this struct by the end of our store refactor.
type K8sGateway struct {
	*gwv1beta1.Gateway
	GatewayState *state.GatewayState

	logger    hclog.Logger
	client    gatewayclient.Client
	config    apigwv1alpha1.GatewayClassConfig
	validator *validator.GatewayValidator
	deployer  *GatewayDeployer
}

var _ store.StatusTrackingGateway = &K8sGateway{}

type K8sGatewayConfig struct {
	ConsulNamespace string
	ConsulCA        string
	SDSHost         string
	SDSPort         int
	State           *state.GatewayState
	Config          apigwv1alpha1.GatewayClassConfig
	Deployer        *GatewayDeployer
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func newK8sGateway(gateway *gwv1beta1.Gateway, config K8sGatewayConfig) *K8sGateway {
	return &K8sGateway{
		Gateway:      gateway,
		GatewayState: config.State,
		config:       config.Config,
		validator:    validator.NewGatewayValidator(config.Client),
		deployer:     config.Deployer,
		logger:       config.Logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace),
		client:       config.Client,
	}
}

func (g *K8sGateway) Validate(ctx context.Context) error {
	return g.validator.Validate(ctx, g.GatewayState, g.Gateway, g.deployer.Service(g.config, g.Gateway))
}

func (g *K8sGateway) ID() core.GatewayID {
	return core.GatewayID{
		Service:         g.Gateway.Name,
		ConsulNamespace: g.GatewayState.ConsulNamespace,
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

	return newBinder(g.client).Bind(ctx, g, k8sRoute)
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

func (g *K8sGateway) CanFetchSecrets(_ context.Context, secrets []string) (bool, error) {
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

func (g *K8sGateway) TrackSync(ctx context.Context, sync func() (bool, error)) error {
	// we've done all but synced our state, so ensure our deployments are up-to-date
	if err := g.deployer.Deploy(ctx, g.GatewayState.ConsulNamespace, g.config, g.Gateway); err != nil {
		return err
	}

	didSync, err := sync()
	if err != nil {
		g.GatewayState.Status.InSync.SyncError = err
	} else if didSync {
		// clear out any old synchronization error statuses
		g.GatewayState.Status.InSync = rstatus.GatewayInSyncStatus{}
	}

	status := g.GatewayState.GetStatus(g.Gateway)
	if !rstatus.GatewayStatusEqual(status, g.Gateway.Status) {
		g.Gateway.Status = status
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(status, "", "  ")
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
