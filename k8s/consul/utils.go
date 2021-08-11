package consul

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func hostnamesForHTTPRoute(listener *gw.Listener, route *gw.HTTPRoute) []string {
	var hostnames []string
	if len(route.Spec.Hostnames) > 1 {
		hostnames = hostnamesToSlice(route.Spec.Hostnames)
	} else if listener.Hostname != nil {
		hostnames = []string{string(*listener.Hostname)}
	}
	// TODO: should a hostname:port value be added if the listener is a non well known port?
	return hostnames
}

// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
const NamespaceNameLabel = "kubernetes.io/metadata.name"

func toNamespaceSet(name string, labels map[string]string) klabels.Labels {
	// If namespace label is not set, implicitly insert it to support older Kubernetes versions
	if labels[NamespaceNameLabel] == name {
		// Already set, avoid copies
		return klabels.Set(labels)
	}
	// First we need a copy to not modify the underlying object
	ret := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		ret[k] = v
	}
	ret[NamespaceNameLabel] = name
	return klabels.Set(ret)
}

func hostnamesToSlice(hostnames []gw.Hostname) []string {
	result := make([]string, len(hostnames))
	for i, h := range hostnames {
		result[i] = string(h)
	}
	return result
}

func kubeProtocolToConsul(protocolType gw.ProtocolType) (proto string, tls bool) {
	switch protocolType {
	case gw.TCPProtocolType, gw.TLSProtocolType:
		return "tcp", protocolType == gw.TLSProtocolType
	case gw.HTTPProtocolType, gw.HTTPSProtocolType:
		return "http", protocolType == gw.HTTPSProtocolType
	case gw.UDPProtocolType:
		return "", false // unsupported
	default:
		return "", false // unknown/unsupported
	}
}

func kubeObjectNamespacedName(o metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}
}
