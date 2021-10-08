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

// SyncError is an error type that can
// be returned by the store to indicate that there was an error
// with external synchronization
type SyncError struct {
	inner error
}

func NewSyncError(err error) SyncError {
	return SyncError{inner: err}
}

func (s SyncError) Error() string {
	return s.inner.Error()
}
