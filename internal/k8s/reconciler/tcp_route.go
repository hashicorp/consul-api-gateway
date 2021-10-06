package reconciler

import "k8s.io/apimachinery/pkg/types"

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}
