package consul

import (
	"context"
	"github.com/hashicorp/consul/api"
)

type Client interface {
	Agent() *api.Agent
	Catalog() *api.Catalog
	ConfigEntries() *api.ConfigEntries
	DiscoveryChain() *api.DiscoveryChain
	Namespaces() *api.Namespaces

	// TODO: drop this
	Internal() *api.Client
}

type client struct {
	client *api.Client
	ctx    context.Context
}

func NewClient(ctx context.Context, c *api.Client) Client {
	return &client{
		ctx:    ctx,
		client: c,
	}
}

func (c *client) Agent() *api.Agent {
	return c.client.Agent()
}

func (c *client) Catalog() *api.Catalog {
	return c.client.Catalog()
}

func (c *client) ConfigEntries() *api.ConfigEntries {
	return c.client.ConfigEntries()
}

func (c *client) DiscoveryChain() *api.DiscoveryChain {
	return c.client.DiscoveryChain()
}

func (c *client) Namespaces() *api.Namespaces {
	return c.client.Namespaces()
}

func (c *client) Internal() *api.Client {
	return c.client
}
