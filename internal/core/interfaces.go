// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package core

import (
	"context"
)

// SyncAdapter is used for synchronizing store state to
// an external system
type SyncAdapter interface {
	Sync(ctx context.Context, gateway ResolvedGateway) (bool, error)
	Clear(ctx context.Context, id GatewayID) error
}
