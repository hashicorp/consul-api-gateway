package utils

import (
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	gatewayGroup = gw.Group(gw.GroupName)
	gatewayKind  = gw.Kind("Gateway")
)

func ReferencesGateway(namespace string, ref gw.ParentRef) (types.NamespacedName, bool) {
	if ref.Group != nil && *ref.Group != gatewayGroup {
		return types.NamespacedName{}, false
	}
	if ref.Kind != nil && *ref.Kind != gatewayKind {
		return types.NamespacedName{}, false
	}

	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return types.NamespacedName{Name: string(ref.Name), Namespace: namespace}, true
}
