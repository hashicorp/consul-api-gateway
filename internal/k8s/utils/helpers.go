package utils

import (
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func NamespacedName(o client.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}
}

func IsFieldUpdated(old, new interface{}) bool {
	return !reflect.DeepEqual(old, new)
}

func HostnamesForHTTPRoute(listenerHostname string, route *gateway.HTTPRoute) []string {
	var hostnames []string
	if len(route.Spec.Hostnames) > 0 {
		hostnames = hostnamesToSlice(route.Spec.Hostnames)
	} else if listenerHostname != "" {
		hostnames = []string{listenerHostname}
	}
	// TODO: should a hostname:port value be added if the listener is a non well known port?
	return hostnames
}

func hostnamesToSlice(hostnames []gateway.Hostname) []string {
	result := make([]string, len(hostnames))
	for i, h := range hostnames {
		result[i] = string(h)
	}
	return result
}
