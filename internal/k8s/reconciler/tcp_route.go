package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}

func convertTCPRoute(namespace, prefix string, meta map[string]string, route *gwv1alpha2.TCPRoute, k8sRoute *K8sRoute) *core.ResolvedRoute {
	name := prefix + route.Name

	resolved := core.NewTCPRouteBuilder().
		WithName(name).
		// we always use the listener namespace for the routes
		// themselves, while the services they route to might
		// be in different namespaces
		WithNamespace(namespace).
		WithMeta(meta).
		WithService(tcpReferencesToService(k8sRoute.references)).
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
