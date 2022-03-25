package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}

func convertTCPRoute(namespace, prefix string, meta map[string]string, route *gw.TCPRoute, state *RouteState) *core.ResolvedRoute {
	name := prefix + route.Name

	resolved := core.NewTCPRouteBuilder().
		WithName(name).
		// we always use the listener namespace for the routes
		// themselves, while the services they route to might
		// be in different namespaces
		WithNamespace(namespace).
		WithMeta(meta).
		WithService(tcpReferencesToService(state.References)).
		Build()
	return &resolved
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
