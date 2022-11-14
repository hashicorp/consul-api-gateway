package consul

import (
	"context"
	"crypto/tls"
	"sync"

	"fmt"
	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type Client interface {
	Agent() *api.Agent
	Catalog() *api.Catalog
	ConfigEntries() *api.ConfigEntries
	DiscoveryChain() *api.DiscoveryChain
	Namespaces() *api.Namespaces

	WatchServers(ctx context.Context) error

	Token() string

	// TODO: drop this
	Internal() *api.Client
}

type ClientConfig struct {
	ApiClientConfig *api.Config
	Addresses       string
	HTTPAddress     string
	HTTPPort        int
	GRPCPort        int
	Namespace       string
	TLS             *tls.Config
	Credentials     discovery.Credentials
	Logger          hclog.Logger
}

type client struct {
	config      ClientConfig
	client      *api.Client
	token       string
	mutex       sync.RWMutex
	initialized chan struct{}
}

func NewClient(config ClientConfig) Client {
	return &client{
		config:      config,
		initialized: make(chan struct{}),
	}
}

func (c *client) wait() {
	<-c.initialized
}

func (c *client) WatchServers(ctx context.Context) error {
	watcher, err := discovery.NewWatcher(ctx, discovery.Config{
		Addresses:   c.config.Addresses,
		GRPCPort:    c.config.GRPCPort,
		TLS:         c.config.TLS,
		Credentials: c.config.Credentials,
	}, c.config.Logger)
	if err != nil {
		return err
	}
	go watcher.Run()
	defer watcher.Stop()

	// Wait for initial state.
	state, err := watcher.State()
	if err != nil {
		return err
	}
	updateClient := func(s discovery.State) error {
		cfg := c.config.ApiClientConfig
		cfg.Namespace = c.config.Namespace
		cfg.Address = fmt.Sprintf("%s:%d", c.config.HTTPAddress, c.config.HTTPPort)
		cfg.Token = s.Token

		client, err := api.NewClient(cfg)
		if err != nil {
			return err
		}

		c.mutex.Lock()
		c.client = client
		c.token = s.Token
		c.mutex.Unlock()

		return nil
	}
	if err := updateClient(state); err != nil {
		return err
	}
	close(c.initialized)

	ch := watcher.Subscribe()
	for {
		select {
		case state := <-ch:
			if err := updateClient(state); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (c *client) Agent() *api.Agent {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Agent()
}

func (c *client) Catalog() *api.Catalog {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Catalog()
}

func (c *client) ConfigEntries() *api.ConfigEntries {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.ConfigEntries()
}

func (c *client) DiscoveryChain() *api.DiscoveryChain {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.DiscoveryChain()
}

func (c *client) Namespaces() *api.Namespaces {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Namespaces()
}

func (c *client) Token() string {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.token
}

func (c *client) Internal() *api.Client {
	c.wait()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client
}
