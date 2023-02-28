// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/hashicorp/consul-api-gateway/internal/consul/mocks"
)

type TestClient struct {
	*api.Client

	peerings *mocks.MockPeerings
}

func NewTestClient(c *api.Client) *TestClient {
	return &TestClient{
		Client: c,
	}
}

func (c *TestClient) WatchServers(ctx context.Context) error {
	return nil
}

func (c *TestClient) Token() string {
	return ""
}

func (c *TestClient) Wait(time.Duration) error {
	return nil
}

func (c *TestClient) Internal() *api.Client {
	return c.Client
}

func (c *TestClient) Peerings() PeeringClient {
	if c.peerings == nil {
		return c.Client.Peerings()
	}
	return c.peerings
}

func (c *TestClient) SetPeerings(peerings *mocks.MockPeerings) {
	c.peerings = peerings
}
