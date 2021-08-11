package consul

import (
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type Client struct {
	*api.Client
	logger hclog.Logger
}

func (c *Client) SetConfigEntries(entries ...api.ConfigEntry) {
	for _, entry := range entries {
		// TODO: handle failures?
		c.logger.Debug("setting entry", "kind", entry.GetKind(), "name", entry.GetName())
		c.ConfigEntries().Set(entry, nil)
	}
}

func (c *Client) DeleteConfigEntries(entries ...api.ConfigEntry) {
	for _, entry := range entries {
		// TODO: handle failures?
		c.logger.Debug("deleting entry", "kind", entry.GetKind(), "name", entry.GetName())
		c.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), nil)
	}
}
