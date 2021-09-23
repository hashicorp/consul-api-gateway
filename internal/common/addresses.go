package common

import (
	"net"
)

// AddressTypeForAddress returns whether envoy should
// treat the given address as a static ip or as a DNS name
func AddressTypeForAddress(address string) string {
	if net.ParseIP(address) != nil {
		return "STATIC"
	}
	return "STRICT_DNS"
}
