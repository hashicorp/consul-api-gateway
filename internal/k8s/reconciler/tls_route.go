package reconciler

import "k8s.io/apimachinery/pkg/types"

func TLSRouteID(namespacedName types.NamespacedName) string {
	return "tls-" + namespacedName.String()
}
