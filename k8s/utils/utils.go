package utils

import (
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func KubeObjectNamespacedName(o client.Object) types.NamespacedName {
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

func IsFieldUpdated(old, new interface{}) bool {
	return !reflect.DeepEqual(old, new)
}
