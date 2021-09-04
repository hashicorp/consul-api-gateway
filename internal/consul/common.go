package consul

import (
	"time"
)

const (
	defaultMaxAttempts     = uint64(30)
	defaultBackoffInterval = 1 * time.Second
)
