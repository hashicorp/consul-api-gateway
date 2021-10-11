package reconciler

import "k8s.io/apimachinery/pkg/types"

func UDPRouteID(namespacedName types.NamespacedName) string {
	return "udp-" + namespacedName.String()
}
