package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"k8s.io/apimachinery/pkg/types"
)

type RouteStatuses map[string]*RouteStatus

func (r *K8sRoute) parentKeyForGateway(parent types.NamespacedName) (string, bool) {
	for _, p := range r.Parents() {
		gatewayName, isGateway := utils.ReferencesGateway(r.GetNamespace(), p)
		if isGateway && gatewayName == parent {
			return asJSON(p), true
		}
	}
	return "", false
}

func (r *K8sRoute) bindFailed(err error, gateway *K8sGateway) {
	id, found := r.parentKeyForGateway(utils.NamespacedName(gateway.Gateway))
	if found {
		r.RouteState.bindFailed(err, id)
	}
}

func (r *K8sRoute) bound(gateway *K8sGateway) {
	id, found := r.parentKeyForGateway(utils.NamespacedName(gateway.Gateway))
	if found {
		r.RouteState.bound(id)
	}
}

func (r *K8sRoute) OnGatewayRemoved(gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.Gateway))
		if found {
			r.RouteState.remove(id)
		}
	}
}
