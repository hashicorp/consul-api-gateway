package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func KubeObjectNamespacedName(o metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}
}

func IsManagedGateway(labels map[string]string) (string, bool) {
	managedBy, ok := labels["managedBy"]
	if !ok || managedBy != "consul-api-gateway" {
		return "", false
	}
	name, ok := labels["name"]
	if !ok {
		return "", false
	}
	return name, true
}
