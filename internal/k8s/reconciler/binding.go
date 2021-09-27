package reconciler

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul/api"
)

const (
	defaultListenerName = "default"
)

type ResolvedGateway struct {
	name      types.NamespacedName
	listeners map[string]*resolvedListener
}

func NewResolvedGateway(name types.NamespacedName) *ResolvedGateway {
	return &ResolvedGateway{
		name:      name,
		listeners: map[string]*resolvedListener{},
	}
}

func (g *ResolvedGateway) AddRoute(lis gw.Listener, route *KubernetesRoute) {
	rLis, ok := g.listeners[string(lis.Name)]
	if !ok {
		rLis = g.newListener(&lis)
	}
	rLis.addRoute(route)
}

func (g *ResolvedGateway) newListener(lis *gw.Listener) *resolvedListener {
	name := defaultListenerName
	if lis.Name != "" {
		name = string(lis.Name)
	}
	var hostname string
	if lis.Hostname != nil {
		hostname = string(*lis.Hostname)
	}

	proto, tls := utils.ProtocolToConsul(lis.Protocol)
	l := &resolvedListener{
		name:     name,
		protocol: proto,
		port:     int(lis.Port),
		tls:      tls,
		hostname: hostname,
	}
	g.listeners[l.name] = l
	return l
}

func (g *ResolvedGateway) computeConfigEntries() (ingress api.ConfigEntry, routers, splitters, defaults *consul.ConfigEntryIndex, err error) {
	igw := &api.IngressGatewayConfigEntry{
		Kind:      api.IngressGateway,
		Name:      g.name.Name,
		Namespace: "", // TODO
		Meta: map[string]string{
			"managed_by":                               "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":      g.name.Name,
			"consul-api-gateway/k8s/Gateway.Namespace": g.name.Namespace,
		},
	}
	computedRouters := consul.NewConfigEntryIndex(api.ServiceRouter)
	computedSplitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	computedDefaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	for _, kubeListener := range g.listeners {
		// kube gateway protocol to ingress gateway conversion
		// Consul uses a separate bool to indicate if the listener has TLS enabled
		// where the gateway api indicates this by the protocol
		listener := &api.IngressListener{
			Port:     kubeListener.port,
			Protocol: kubeListener.protocol,
			// TODO: TLS
		}

		// TODO support other route types
		httpServices, httpRouters, httpSplitters, httpDefaults := kubeListener.computeDiscoveryChain(g.name)
		listener.Services = append(listener.Services, httpServices...)
		computedRouters.Merge(httpRouters)
		computedSplitters.Merge(httpSplitters)
		computedDefaults.Merge(httpDefaults)
		if len(listener.Services) > 0 {
			igw.Listeners = append(igw.Listeners, *listener)
		}
	}

	return igw, computedRouters, computedSplitters, computedDefaults, nil
}

type resolvedListener struct {
	name              string
	protocol          string
	port              int
	tls               bool
	hostname          string
	httpRouteBindings []*gw.HTTPRoute
}

func (l *resolvedListener) addRoute(route *KubernetesRoute) {
	if route.IsHTTPRoute() {
		if httpRoute, ok := route.AsHTTPRoute(); ok {
			l.httpRouteBindings = append(l.httpRouteBindings, httpRoute.DeepCopy())
		}
	}
}

func (l *resolvedListener) computeDiscoveryChain(gateway types.NamespacedName) (services []api.IngressService, routers, splitters, defaults *consul.ConfigEntryIndex) {
	routers = consul.NewConfigEntryIndex(api.ServiceRouter)
	splitters = consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults = consul.NewConfigEntryIndex(api.ServiceDefaults)
	for _, kubeRoute := range l.httpRouteBindings {
		meta := map[string]string{
			"managed_by":                                 "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":        gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace":   gateway.Namespace,
			"consul-api-gateway/k8s/HTTPRoute.Name":      kubeRoute.Name,
			"consul-api-gateway/k8s/HTTPRoute.Namespace": kubeRoute.Namespace,
		}
		prefix := fmt.Sprintf("consul-api-gateway_%s_", gateway.Name)
		router, splits := HTTPRouteToServiceDiscoChain(kubeRoute, prefix, meta)

		services = append(services, api.IngressService{
			Name:      router.Name,
			Hosts:     utils.HostnamesForHTTPRoute(l.hostname, kubeRoute),
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
