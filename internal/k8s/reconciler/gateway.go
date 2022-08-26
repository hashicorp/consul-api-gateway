package reconciler

import (
	"context"
	"encoding/json"
	"strings"

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

type K8sGateway struct {
	*gwv1beta1.Gateway
	GatewayState *state.GatewayState

	logger    hclog.Logger
	client    gatewayclient.Client
	config    apigwv1alpha1.GatewayClassConfig
	validator *validator.GatewayValidator
	deployer  *GatewayDeployer

	listeners []*K8sListener
}

var _ store.StatusTrackingGateway = &K8sGateway{}

type K8sGatewayConfig struct {
	ConsulNamespace string
	ConsulCA        string
	SDSHost         string
	SDSPort         int
	Config          apigwv1alpha1.GatewayClassConfig
	Deployer        *GatewayDeployer
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func newK8sGateway(gateway *gwv1beta1.Gateway, config K8sGatewayConfig) *K8sGateway {
	// FUTURE (nathancoleman) See if we can avoid setting ConsulNamespace out of band
	gwState := state.InitialGatewayState(gateway)
	gwState.ConsulNamespace = config.ConsulNamespace

	gatewayLogger := config.Logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)
	listeners := make([]*K8sListener, 0, len(gateway.Spec.Listeners))
	for index, listener := range gateway.Spec.Listeners {
		k8sListener := NewK8sListener(gateway, listener, K8sListenerConfig{
			ConsulNamespace: config.ConsulNamespace,
			Logger:          gatewayLogger,
			Client:          config.Client,
			State:           gwState.Listeners[index],
		})
		k8sListener.status = &(gwState.Listeners[index].Status)
		listeners = append(listeners, k8sListener)
	}

	return &K8sGateway{
		Gateway:      gateway,
		GatewayState: gwState,
		config:       config.Config,
		validator:    validator.NewGatewayValidator(config.Client),
		deployer:     config.Deployer,
		logger:       gatewayLogger,
		client:       config.Client,
		listeners:    listeners,
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

func (g *K8sGateway) Listeners() []store.Listener {
	listeners := []store.Listener{}

	for _, listener := range g.listeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

func (g *K8sGateway) ShouldUpdate(other store.Gateway) bool {
	if other == nil {
		return false
	}

	if g == nil {
		return true
	}

	otherGateway, ok := other.(*K8sGateway)
	if !ok {
		return false
	}

	return !utils.ResourceVersionGreater(g.Gateway.ResourceVersion, otherGateway.Gateway.ResourceVersion)
}

func (g *K8sGateway) ShouldBind(route store.Route) bool {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false
	}

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.Gateway) == namespacedName {
				return true
			}
		}
	}
	return false
}

func (g *K8sGateway) Status() gwv1beta1.GatewayStatus {
	return g.GatewayState.GetStatus(g.Gateway)
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

	status := g.Status()
	if !gatewayStatusEqual(status, g.Gateway.Status) {
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

func setToCSV(set map[string]struct{}) string {
	values := []string{}
	for value := range set {
		values = append(values, value)
	}
	return strings.Join(values, ", ")
}
