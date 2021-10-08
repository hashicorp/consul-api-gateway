package core

import (
	"context"
)

// SyncAdapter is used for synchronizing store state to
// an external system
type SyncAdapter interface {
	Sync(ctx context.Context, gateway ResolvedGateway) error
	Clear(ctx context.Context, id GatewayID) error
}
