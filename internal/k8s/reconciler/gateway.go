package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/builder"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

type K8sGateway struct {
	*GatewayState
	*gw.Gateway

	listeners []*K8sListener

	consulNamespace   string
	logger            hclog.Logger
	client            gatewayclient.Client
	config            apigwv1alpha1.GatewayClassConfig
	deploymentBuilder builder.DeploymentBuilder
	serviceBuilder    builder.ServiceBuilder
}

var _ store.StatusTrackingGateway = &K8sGateway{}

// TODO: remove this
func (g *K8sGateway) SetState(state *GatewayState) {
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

	boundListeners := []string{}
	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.Gateway) == namespacedName {
				for _, l := range g.listeners {
					if (&Binder{
						Client:        l.client,
						Gateway:       l.gateway,
						Listener:      l.listener,
						ListenerState: l.ListenerState,
					}).Bind(ctx, k8sRoute) {
						atomic.AddInt32(&l.ListenerState.RouteCount, 1)
						boundListeners = append(boundListeners, l.ID())
					}
				}
				return boundListeners
			}
		}
	}
	return nil
}

func (g *K8sGateway) TrackSync(ctx context.Context, sync func() (bool, error)) error {
	// we've done all but synced our state, so ensure our deployments are up-to-date
	if err := g.ensureDeploymentExists(ctx); err != nil {
		return err
	}

	didSync, err := sync()
	if err != nil {
		g.GatewayState.Status.InSync.SyncError = err
	} else if didSync {
		// clear out any old synchronization error statuses
		g.GatewayState.Status.InSync = GatewayInSyncStatus{}
	}

	status := g.GetStatus(g.Gateway)
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

func (g *K8sGateway) ensureDeploymentExists(ctx context.Context) error {
	// Create service account for the gateway
	if serviceAccount := g.config.ServiceAccountFor(g.Gateway); serviceAccount != nil {
		if err := g.client.EnsureServiceAccount(ctx, g.Gateway, serviceAccount); err != nil {
			return err
		}
	}

	deployment := g.deploymentBuilder.Build()
	mutated := deployment.DeepCopy()
	if updated, err := g.client.CreateOrUpdateDeployment(ctx, mutated, func() error {
		mutated = apigwv1alpha1.MergeDeployment(deployment, mutated)
		return g.client.SetControllerOwnership(g.Gateway, mutated)
	}); err != nil {
		return fmt.Errorf("failed to create or update gateway deployment: %w", err)
	} else if updated {
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(mutated, "", "  ")
			if err == nil {
				g.logger.Trace("created or updated gateway deployment", "deployment", string(data))
			}
		}
	}

	// Create service for the gateway
	if service := g.serviceBuilder.Build(); service != nil {
		mutated := service.DeepCopy()
		if updated, err := g.client.CreateOrUpdateService(ctx, mutated, func() error {
			mutated = apigwv1alpha1.MergeService(service, mutated)
			return g.client.SetControllerOwnership(g.Gateway, mutated)
		}); err != nil {
			return fmt.Errorf("failed to create or update gateway service: %w", err)
		} else if updated {
			if g.logger.IsTrace() {
				data, err := json.MarshalIndent(mutated, "", "  ")
				if err == nil {
					g.logger.Trace("created or updated gateway service", "service", string(data))
				}
			}
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
