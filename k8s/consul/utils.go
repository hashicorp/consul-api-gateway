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

func routeMatches(gateway *gw.Gateway, selector gw.RouteBindingSelector, route *gw.HTTPRoute) (bool, string) {
	if selector.Kind != route.Kind {
		return false, "selector and route Kind do not match"
	}
	// TODO check selector group?

	// check gateway labels
	var labelSelector klabels.Selector
	var err error
	if selector.Selector == nil {
		labelSelector = klabels.Everything()
	} else {
		labelSelector, err = metav1.LabelSelectorAsSelector(selector.Selector)
		if err != nil {
			return false, "bad selector"
		}
	}

	if !labelSelector.Matches(klabels.Set(route.Labels)) {
		return false, "gateway labels selector does not match route"
	}

	// check gateway namespace
	namespaceSelector := selector.Namespaces
	// set default is namespace selector is nil
	from := gw.RouteSelectSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gw.RouteSelectAll:
		// matches continue
	case gw.RouteSelectSame:
		if gateway.Namespace != route.Namespace {
			return false, "gateway namespace does not match route"
		}
	case gw.RouteSelectSelector:
		ns, err := metav1.LabelSelectorAsSelector(namespaceSelector.Selector)
		if err != nil {
			return false, "bad selector"
		}

		if !ns.Matches(toNamespaceSet(route.Namespace, route.Labels)) {
			return false, "gateway namespace does not match route namespace selector"
		}

	}

	// check gateway selector
	gatewaySelector := route.Spec.Gateways
	allow := gw.GatewayAllowSameNamespace
	if gatewaySelector != nil && gatewaySelector.Allow != nil {
		allow = *gatewaySelector.Allow
	}

	switch allow {
	case gw.GatewayAllowAll:
		// matches
	case gw.GatewayAllowFromList:
		found := false
		if gatewaySelector == nil {
			return false, "route gateway selector is empty but gateway requires allow from list"
		}
		for _, gw := range gatewaySelector.GatewayRefs {
			if gw.Name == gateway.Name && gw.Namespace == gateway.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false, "route gateway selector did not match"
		}
	case gw.GatewayAllowSameNamespace:
		if gateway.Namespace != route.Namespace {
			return false, "gateway namespace does not match and is required by gateway selector"
		}
	}

	return true, ""
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
