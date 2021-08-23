package consul

import (
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func hostnamesForHTTPRoute(listenerHostname string, route *gw.HTTPRoute) []string {
	var hostnames []string
	if len(route.Spec.Hostnames) > 1 {
		hostnames = hostnamesToSlice(route.Spec.Hostnames)
	} else if listenerHostname != "" {
		hostnames = []string{listenerHostname}
	}
	// TODO: should a hostname:port value be added if the listener is a non well known port?
	return hostnames
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
