// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func ProtocolToConsul(protocolType gwv1beta1.ProtocolType) (proto string, tls bool) {
	switch protocolType {
	case gwv1beta1.TCPProtocolType, gwv1beta1.TLSProtocolType:
		return "tcp", protocolType == gwv1beta1.TLSProtocolType
	case gwv1beta1.HTTPProtocolType, gwv1beta1.HTTPSProtocolType:
		return "http", protocolType == gwv1beta1.HTTPSProtocolType
	case gwv1beta1.UDPProtocolType:
		return "", false // unsupported
	default:
		return "", false // unknown/unsupported
	}
}
