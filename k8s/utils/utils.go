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

func IsFieldUpdated(old, new interface{}) bool {
	return !reflect.DeepEqual(old, new)
}
