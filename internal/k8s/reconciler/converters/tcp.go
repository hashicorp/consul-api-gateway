package converters

import (
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type TCPRouteConverter struct {
	namespace string
	hostname  string
	meta      map[string]string
	route     *gw.TCPRoute
	state     *state.RouteState
}

type TCPRouteConverterConfig struct {
	Namespace string
	Hostname  string
	Prefix    string
	Meta      map[string]string
	Route     *gw.TCPRoute
	State     *state.RouteState
}

func NewTCPRouteConverter(config TCPRouteConverterConfig) *TCPRouteConverter {
	return &TCPRouteConverter{
		namespace: config.Namespace,
		hostname:  config.Hostname,
		meta:      config.Meta,
		route:     config.Route,
		state:     config.State,
	}
}

func (c *TCPRouteConverter) Convert() core.ResolvedRoute {
	return core.NewTCPRouteBuilder().
		WithName(c.route.Name).
		// we always use the listener namespace for the routes
		// themselves, while the services they route to might
		// be in different namespaces
		WithNamespace(c.namespace).
		WithMeta(c.meta).
		WithService(tcpReferencesToService(c.state.References)).
		Build()
}

func tcpReferencesToService(referenceMap service.RouteRuleReferenceMap) core.ResolvedService {
	for _, references := range referenceMap {
		for _, reference := range references {
			switch reference.Type {
			case service.ConsulServiceReference:
				// at this point there should only be a single resolved service in the reference map
				return core.ResolvedService{
					ConsulNamespace: reference.Consul.Namespace,
					Service:         reference.Consul.Name,
				}
			default:
				// TODO: support other reference types
				continue
			}
		}
	}
	return core.ResolvedService{}
}
