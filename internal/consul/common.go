package consul

import (
	"net"
	"time"
)

const (
	defaultMaxAttempts     = uint64(30)
	defaultBackoffInterval = 1 * time.Second
)

func addressTypeForAddress(address string) string {
	if net.ParseIP(address) != nil {
		return "STATIC"
	}
	return "STRICT_DNS"
}
