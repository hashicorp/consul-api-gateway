package consul

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

// GatewayReconcileManager manages a GatewayReconciler for each Gateway and is the interface by which Consul operations
// should be invoked in a kubernetes controller.
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
	m.logger.Debug("gateway upserted", "gateway", g)
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
	m.logger.Debug("route upserted", "route", r)
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

	consul ConfigEntriesClient

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
		signalReconcileCh: make(chan struct{}, 1), // buffered chan allow for a single pending reconcile signal
		stopReconcileCh:   make(chan struct{}, 0),
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
				if err := c.reconcile(); err != nil {
					c.logger.Error("failed to reconcile gateway", "error", err)
				}
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

// reconcile should never be called outside of loop() to ensure it is not invoked concurrently
func (c *GatewayReconciler) reconcile() error {
	if c.logger.IsTrace() {
		start := time.Now()
		c.logger.Trace("reconcile started")
		defer c.logger.Trace("reconcile finished", "duration", hclog.Fmt("%dms", time.Now().Sub(start).Milliseconds()))
	}

	igw, computedRouters, computedSplitters, computedDefaults, err := computeConfigEntries(c.kubeGateway, c.kubeHTTPRoutes)
	if err != nil {
		return fmt.Errorf("reconcile failed: %w", err)
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

func computeConfigEntries(gateway *gw.Gateway, routes *KubernetesRoutes) (ingress api.ConfigEntry, routers *ConfigEntryIndex, splitters *ConfigEntryIndex, defaults *ConfigEntryIndex, err error) {
	igw := &api.IngressGatewayConfigEntry{
		Kind:      api.IngressGateway,
		Name:      gateway.Name,
		Namespace: "", // TODO
		Meta: map[string]string{
			"managed_by":                  "polar",
			"polar/k8s/Gateway.Name":      gateway.Name,
			"polar/k8s/Gateway.Namespace": gateway.Namespace,
		},
	}
	computedRouters := NewConfigEntryIndex(api.ServiceRouter)
	computedSplitters := NewConfigEntryIndex(api.ServiceSplitter)
	computedDefaults := NewConfigEntryIndex(api.ServiceDefaults)

	portListenerIdx := map[int]*api.IngressListener{}
	for _, kubeListener := range gateway.Spec.Listeners {
		// kube gateway protocol to ingress gateway conversion
		// Consul uses a separate bool to indicate if the listener has TLS enabled
		// where the gateway api indicates this by the protocol
		proto, tls := kubeProtocolToConsul(kubeListener.Protocol)
		_ = tls // TODO: Remove once TLS is configurable at IngressListener level
		if proto == "" {
			err = fmt.Errorf("unsupported listener protocol %q", kubeListener.Protocol)
			return
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
				err = fmt.Errorf("multiple listeners with the same port but different protocols are not supported")
				return
			}
		}

		// TODO support other route types
		httpServices, httpRouters, httpSplitters, httpDefaults := computeConfigEntriesForHTTPRoutes(types.NamespacedName{Name: gateway.Name, Namespace: gateway.Namespace}, &kubeListener, routes.HTTPRoutes())
		listener.Services = append(listener.Services, httpServices...)
		computedRouters.Merge(httpRouters)
		computedSplitters.Merge(httpSplitters)
		computedDefaults.Merge(httpDefaults)
	}

	for _, listener := range portListenerIdx {
		if len(listener.Services) > 0 {
			igw.Listeners = append(igw.Listeners, *listener)
		}
	}

	return igw, computedRouters, computedSplitters, computedDefaults, nil
}

func computeConfigEntriesForHTTPRoutes(gateway types.NamespacedName, listener *gw.Listener, routes []*gw.HTTPRoute) (services []api.IngressService, routers *ConfigEntryIndex, splitters *ConfigEntryIndex, defaults *ConfigEntryIndex) {
	routers = NewConfigEntryIndex(api.ServiceRouter)
	splitters = NewConfigEntryIndex(api.ServiceSplitter)
	defaults = NewConfigEntryIndex(api.ServiceDefaults)
	for _, kubeRoute := range routes {
		match, res := routeMatches(gateway, listener.Routes, kubeRoute)
		fmt.Println(res)
		if !match {
			continue
		}
		meta := map[string]string{
			"managed_by":                    "polar",
			"polar/k8s/Gateway.Name":        gateway.Name,
			"polar/k8s/Gateway.Namespace":   gateway.Namespace,
			"polar/k8s/HTTPRoute.Name":      kubeRoute.Name,
			"polar/k8s/HTTPRoute.Namespace": kubeRoute.Namespace,
		}
		prefix := fmt.Sprintf("polar_%s_", gateway.Name)
		router, splits := HTTPRouteToServiceDiscoChain(kubeRoute, prefix, meta)

		services = append(services, api.IngressService{
			Name:      router.Name,
			Hosts:     hostnamesForHTTPRoute(listener, kubeRoute),
			Namespace: "", // TODO
		})
		routers.Add(router)
		svcDefault := httpServiceDefault(router, meta)
		defaults.Add(svcDefault)
		for _, split := range splits {
			splitters.Add(split)
			if split.Name != svcDefault.Name {
				defaults.Add(httpServiceDefault(split, meta))
			}
		}
	}
	return
}
