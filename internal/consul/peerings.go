// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"

	"github.com/hashicorp/consul/api"
)

//go:generate mockgen -source ./peerings.go -destination ./mocks/peerings.go -package mocks Peerings

type Peerings interface {
	Read(context.Context, string, *api.QueryOptions) (*api.Peering, *api.QueryMeta, error)
}
