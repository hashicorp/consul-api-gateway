package testing

import (
	"context"
	"time"

	"github.com/hashicorp/consul/api"
)

type TestClient struct {
	*api.Client
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
