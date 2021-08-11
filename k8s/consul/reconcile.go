package consul

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type GatewayReconcileManager struct {
	ctx         context.Context
	consul      *api.Client
	reconcilers map[types.NamespacedName]*GatewayReconciler
	routes      *KubernetesRoutes
	logger      hclog.Logger
}

func NewReconcileManager(ctx context.Context, consul *api.Client, logger hclog.Logger) *GatewayReconcileManager {
	return &GatewayReconcileManager{
		ctx:         ctx,
		consul:      consul,
		reconcilers: map[types.NamespacedName]*GatewayReconciler{},
		routes:      NewKubernetesRoutes(),
		logger:      logger,
	}
}

func (m *GatewayReconcileManager) UpsertGateway(g *gw.Gateway) {
	m.logger.Info("gateway upserted", "gateway", g)
	namespacedName := kubeObjectNamespacedName(g)
	r, ok := m.reconcilers[namespacedName]
	if !ok {
		r = newReconcilerForGateway(m.ctx, m.consul, m.logger, g, m.routes)
		go r.loop()
		m.reconcilers[namespacedName] = r
	}

	r.kubeGateway = g
	r.signalReconcile()
}

func (m *GatewayReconcileManager) UpsertHTTPRoute(r *gw.HTTPRoute) {
	m.logger.Info("route upserted", "route", r)
	m.routes.Set(r)
	m.signalAll()
}

func (m *GatewayReconcileManager) DeleteGateway(name types.NamespacedName) {
	r, ok := m.reconcilers[name]
	if !ok {
		return
	}

	r.stop()
	delete(m.reconcilers, name)
}

func (m *GatewayReconcileManager) DeleteRoute(name types.NamespacedName) {
	m.routes.Delete(name)
	m.signalAll()
}

func (m *GatewayReconcileManager) signalAll() {
	for _, r := range m.reconcilers {
		r.signalReconcile()
	}
}

type GatewayReconciler struct {
	ctx               context.Context
	signalReconcileCh chan struct{}
	stopReconcileCh   chan struct{}

	consul *Client

	kubeGateway    *gw.Gateway
	kubeHTTPRoutes *KubernetesRoutes

	routers   *ConfigEntryIndex
	splitters *ConfigEntryIndex
	defaults  *ConfigEntryIndex

	logger hclog.Logger
}

func newReconcilerForGateway(ctx context.Context, c *api.Client, logger hclog.Logger, kubeGateway *gw.Gateway, routes *KubernetesRoutes) *GatewayReconciler {
	logger = logger.With("gateway", kubeGateway.Name, "namespace", kubeGateway.Namespace)
	return &GatewayReconciler{
		ctx:               ctx,
		signalReconcileCh: make(chan struct{}, 1),
		stopReconcileCh:   make(chan struct{}, 1),
		consul:            &Client{Client: c, logger: logger},
		kubeGateway:       kubeGateway,
		kubeHTTPRoutes:    routes,

		routers:   NewConfigEntryIndex(api.ServiceRouter),
		splitters: NewConfigEntryIndex(api.ServiceSplitter),
		defaults:  NewConfigEntryIndex(api.ServiceDefaults),
		logger:    logger,
	}
}

func (c *GatewayReconciler) signalReconcile() {
	select {
	case c.signalReconcileCh <- struct{}{}:
	default:
	}
}

func (c *GatewayReconciler) loop() {
	for {
		select {
		case <-c.signalReconcileCh:
			// make sure theres no pending stop signal before starting a reconcile
			// this can happen if both chans are sending as selection of cases is non deterministic
			select {
			case <-c.stopReconcileCh:
				return
			default:
				c.reconcile()
			}
		case <-c.ctx.Done():
			return
		case <-c.stopReconcileCh:
			return
		}
	}
}

func (c *GatewayReconciler) stop() {
	c.stopReconcileCh <- struct{}{}
}

func (c *GatewayReconciler) reconcile() error {
	c.logger.Trace("reconcile started")
	igw := &api.IngressGatewayConfigEntry{
		Kind:      api.IngressGateway,
		Name:      c.kubeGateway.Name,
		Namespace: "", // TODO
		Meta: map[string]string{
			"managed_by": "polar",
		},
	}

	computedRouters := NewConfigEntryIndex(api.ServiceRouter)
	computedSplitters := NewConfigEntryIndex(api.ServiceSplitter)
	computedDefaults := NewConfigEntryIndex(api.ServiceDefaults)

	portListenerIdx := map[int]*api.IngressListener{}
	for _, kubeListener := range c.kubeGateway.Spec.Listeners {
		// kube gateway protocol to ingress gateway conversion
		// Consul uses a separate bool to indicate if the listener has TLS enabled
		// where the gateway api indicates this by the protocol
		proto, tls := kubeProtocolToConsul(kubeListener.Protocol)
		_ = tls // TODO: Remove once TLS is configurable at IngressListener level
		if proto == "" {
			return fmt.Errorf("unsupported listener protocol %q", kubeListener.Protocol)
		}
		port := int(kubeListener.Port)
		listener, ok := portListenerIdx[port]
		if !ok {
			listener = &api.IngressListener{
				Port:     int(kubeListener.Port),
				Protocol: proto,
				// TODO: TLS
			}
			portListenerIdx[port] = listener
		} else {
			if listener.Protocol != proto {
				return fmt.Errorf("multiple listeners with the same port but different protocols are not supported")
			}
		}

		// TODO support other route types
		for _, kubeRoute := range c.kubeHTTPRoutes.HTTPRoutes() {
			match, reason := routeMatches(c.kubeGateway, kubeListener.Routes, kubeRoute)
			c.logger.Trace("route checked", "route", kubeRoute.Name, "matched", match, "reason", reason)
			if !match {
				continue
			}
			prefix := fmt.Sprintf("polar_%s_", c.kubeGateway.Name)
			router, splits := HTTPRouteToServiceDiscoChain(kubeRoute, prefix)

			listener.Services = append(listener.Services, api.IngressService{
				Name:      router.Name,
				Hosts:     hostnamesForHTTPRoute(&kubeListener, kubeRoute),
				Namespace: "", // TODO
			})
			computedRouters.Add(router)
			defaults := httpServiceDefault(router)
			computedDefaults.Add(defaults)
			for _, split := range splits {
				computedSplitters.Add(split)
				if split.Name != defaults.Name {
					computedDefaults.Add(httpServiceDefault(split))
				}
			}
		}

		if len(listener.Services) > 0 {
			igw.Listeners = append(igw.Listeners, *listener)
		}
	}

	// Since we can't make multiple config entry changes in a single transaction we must
	// perform the operations in a set that is least likely to induce downtime.
	// First the new service-defaults, routers and splitters should be set
	// Second the ingress gateway
	// Third the removal of any service-defaults, routers or splitters that no longer exist
	// TODO: what happens if we get an error here? we could leak config entries if we get an error on removal, maybe they should get garbage collected by polar?

	c.consul.SetConfigEntries(computedRouters.ToArray()...)
	c.consul.SetConfigEntries(computedSplitters.ToArray()...)
	c.consul.SetConfigEntries(computedDefaults.ToArray()...)

	c.consul.SetConfigEntries(igw)

	c.consul.DeleteConfigEntries(computedRouters.Difference(c.routers).ToArray()...)
	c.consul.DeleteConfigEntries(computedSplitters.Difference(c.splitters).ToArray()...)
	c.consul.DeleteConfigEntries(computedDefaults.Difference(c.defaults).ToArray()...)

	c.routers = computedRouters
	c.splitters = computedSplitters
	c.defaults = computedDefaults
	return nil
}
