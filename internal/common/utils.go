package common

import "net"

func AddressTypeForAddress(address string) string {
	if net.ParseIP(address) != nil {
		return "STATIC"
	}
	return "STRICT_DNS"
}
