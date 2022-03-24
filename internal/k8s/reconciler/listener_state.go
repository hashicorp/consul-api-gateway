package reconciler

import "github.com/hashicorp/consul-api-gateway/internal/core"

// ListenerState holds ephemeral state for listeners
type ListenerState struct {
	RouteCount int32
	Status     ListenerStatus
	TLS        core.TLSParams
}
