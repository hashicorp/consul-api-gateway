package utils

import (
	"reflect"
	"strings"

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

// HostnamesMatch checks listener and route hostnames based off of the Gateway Spec. The spec states that a route
// cannot be admitted unless one of the hostnames match the listener's hostname. If the route or listener hostnames are not
// set, '*' is assumed which allows all.
func HostnamesMatch(a, b gateway.Hostname) bool {
	if a == "" || a == "*" || b == "" || b == "*" {
		// any wildcard always matches
		return true
	}

	if strings.HasPrefix(string(a), "*.") || strings.HasPrefix(string(b), "*.") {
		aLabels, bLabels := strings.Split(string(a), "."), strings.Split(string(b), ".")
		if len(aLabels) != len(bLabels) {
			return false
		}

		for i := 1; i < len(aLabels); i++ {
			if !strings.EqualFold(aLabels[i], bLabels[i]) {
				return false
			}
		}
		return true
	}

	return a == b
}
