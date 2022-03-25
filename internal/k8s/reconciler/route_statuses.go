package reconciler

import (
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func (r *K8sRoute) parentKeyForGateway(parent types.NamespacedName) (string, bool) {
	for _, p := range r.Parents() {
		gatewayName, isGateway := utils.ReferencesGateway(r.GetNamespace(), p)
		if isGateway && gatewayName == parent {
			return common.AsJSON(p), true
		}
	}
	return "", false
}

func (r *K8sRoute) bindFailed(err error, ref gw.ParentRef) {
	r.ParentStatuses.BindFailed(r.ResolutionErrors, err, common.AsJSON(ref))
}

func (r *K8sRoute) OnGatewayRemoved(gateway store.Gateway) {
	k8sGateway, ok := gateway.(*K8sGateway)
	if ok {
		id, found := r.parentKeyForGateway(utils.NamespacedName(k8sGateway.Gateway))
		if found {
			r.ParentStatuses.Remove(id)
		}
	}
}
