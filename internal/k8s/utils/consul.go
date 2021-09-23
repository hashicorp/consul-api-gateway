package utils

import (
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func ProtocolToConsul(protocolType gateway.ProtocolType) (proto string, tls bool) {
	switch protocolType {
	case gateway.TCPProtocolType, gateway.TLSProtocolType:
		return "tcp", protocolType == gateway.TLSProtocolType
	case gateway.HTTPProtocolType, gateway.HTTPSProtocolType:
		return "http", protocolType == gateway.HTTPSProtocolType
	case gateway.UDPProtocolType:
		return "", false // unsupported
	default:
		return "", false // unknown/unsupported
	}
}
