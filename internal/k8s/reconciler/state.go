package reconciler

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type State struct {
	logger         hclog.Logger
	client         gatewayclient.Client
	consul         *api.Client
	registry       *common.GatewaySecretRegistry
	controllerName string

	gateways map[types.NamespacedName]*BoundGateway
	routes   map[types.NamespacedName]*K8sRoute
	mutex    sync.RWMutex

	activeGateways int64
}

type StateConfig struct {
	ControllerName string
	Registry       *common.GatewaySecretRegistry
	Consul         *api.Client
	Client         gatewayclient.Client
	Logger         hclog.Logger
}

func NewState(config StateConfig) *State {
	return &State{
		logger:         config.Logger,
		client:         config.Client,
		consul:         config.Consul,
		registry:       config.Registry,
		controllerName: config.ControllerName,
		routes:         make(map[types.NamespacedName]*K8sRoute),
		gateways:       make(map[types.NamespacedName]*BoundGateway),
	}
}

func (g *State) syncGateways(ctx context.Context) error {
	var syncGroup multierror.Group

	for _, gw := range g.gateways {
		gateway := gw
		syncGroup.Go(func() error {
			return gateway.Sync(ctx, g.consul)
		})
	}
	return syncGroup.Wait()
}

func (g *State) syncRouteStatuses(ctx context.Context) error {
	var syncGroup multierror.Group

	for _, r := range g.routes {
		route := r
		syncGroup.Go(func() error {
			return g.client.UpdateStatus(ctx, route.Route)
		})
	}
	return syncGroup.Wait()
}

func (g *State) sync(ctx context.Context) error {
	var syncGroup multierror.Group

	syncGroup.Go(func() error {
		return g.syncGateways(ctx)
	})
	syncGroup.Go(func() error {
		return g.syncRouteStatuses(ctx)
	})
	return syncGroup.Wait()
}

func (g *State) DeleteRoute(ctx context.Context, namespacedName types.NamespacedName) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, gateway := range g.gateways {
		gateway.Remove(namespacedName)
	}

	// sync the gateways to consul and route statuses to k8s
	return g.sync(ctx)
}

func (g *State) AddRoute(ctx context.Context, route *K8sRoute) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	namespacedName := utils.NamespacedName(route)
	current, found := g.routes[namespacedName]
	if found {
		if current.GetGeneration() > route.GetGeneration() {
			// we have an old route, ignore it
			return nil
		}
	}

	g.routes[namespacedName] = route

	// resolve any service references for the route
	if err := route.ResolveReferences(ctx, g.client, g.consul); err != nil {
		return err
	}

	// bind to gateways
	for _, gateway := range g.gateways {
		if err := gateway.Bind(route); err != nil {
			return err
		}
	}

	// sync the gateways to consul and route statuses to k8s
	return g.sync(ctx)
}

func (g *State) AddGateway(ctx context.Context, gw *gw.Gateway) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	namespacedName := utils.NamespacedName(gw)

	current, found := g.gateways[namespacedName]
	if found {
		if current.gateway.Generation > gw.Generation {
			// we have an old gateway, ignore it
			return nil
		}
	}

	updated, err := NewBoundGateway(ctx, g.controllerName, g.client, gw, current)
	if err != nil {
		// we had an issue resolving listener references
		return err
	}

	if !found {
		// this was an insert
		activeGateways := atomic.AddInt64(&g.activeGateways, 1)
		metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))
		g.registry.AddGateway(common.GatewayInfo{
			Service:   gw.Name,
			Namespace: gw.Namespace,
		}, referencedSecretsForGateway(gw)...)
		g.logger.Trace("gateway inserted", "gateway", gw.Name)
	}

	g.gateways[namespacedName] = updated

	// bind routes to this gateway
	for _, route := range g.routes {
		if err := updated.Bind(route); err != nil {
			return err
		}
	}

	// sync the gateways to consul and route statuses to k8s
	return g.sync(ctx)
}

func (g *State) DeleteGateway(ctx context.Context, namespacedName types.NamespacedName) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	_, found := g.gateways[namespacedName]
	if !found {
		return nil
	}

	// deregistration of the service in the gateway
	// handles resource cleanup, we can just remove
	// it from being tracked and sync back route statuses
	for _, route := range g.routes {
		// remove all status references
		route.SetStatus(clearParentStatus(route.GetName(), route.RouteStatus(), namespacedName))
	}
	delete(g.gateways, namespacedName)

	g.registry.RemoveGateway(common.GatewayInfo{
		Service:   namespacedName.Name,
		Namespace: namespacedName.Namespace,
	})
	activeGateways := atomic.AddInt64(&g.activeGateways, -1)
	metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))

	// sync route statuses to k8s
	return g.syncRouteStatuses(ctx)
}

func referencedSecretsForGateway(g *gw.Gateway) []string {
	secrets := []string{}
	for _, listener := range g.Spec.Listeners {
		if listener.TLS != nil {
			ref := listener.TLS.CertificateRef
			if ref != nil {
				n := ref.Namespace
				namespace := "default"
				if n != nil {
					namespace = string(*n)
				}
				secrets = append(secrets, utils.NewK8sSecret(namespace, ref.Name).String())
			}
		}
	}
	return secrets
}
