package consul

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"time"

	"fmt"

	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

var (
	// Calling discovery.NewWatcher registers a new gRPC load balancer
	// type tied to the consul:// scheme, which calls the global
	// google.golang.org/grpc/balancer.Register, which, as specified
	// in their docs is not threadsafe and should be called only in an
	// init function. This mutex makes it so we can boot up multiple watchers
	// particularly in our tests.
	globalWatcherMutex sync.Mutex
)

type Client interface {
	Agent() *api.Agent
	ACL() *api.ACL
	Catalog() *api.Catalog
	ConfigEntries() *api.ConfigEntries
	DiscoveryChain() *api.DiscoveryChain
	Namespaces() *api.Namespaces

	WatchServers(ctx context.Context) error

	Token() string
	Wait(until time.Duration) error

	// TODO: drop this
	Internal() *api.Client
}

type ClientConfig struct {
	ApiClientConfig *api.Config
	PlainText       bool
	Addresses       string
	HTTPPort        int
	GRPCPort        int
	TLS             *tls.Config
	Credentials     discovery.Credentials
	Logger          hclog.Logger
}

type client struct {
	stop        func()
	config      ClientConfig
	client      *api.Client
	token       string
	mutex       sync.RWMutex
	initialized chan error
}

func NewClient(config ClientConfig) Client {
	config.Logger = hclog.Default()
	return &client{
		config:      config,
		initialized: make(chan error, 1),
	}
}

func (c *client) Wait(until time.Duration) error {
	select {
	case err := <-c.initialized:
		return err
	case <-time.After(until):
		c.stop()
		return errors.New("did not get state within time limit")
	}
}

func (c *client) WatchServers(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.stop = cancel

	var static bool
	serverName := c.config.Addresses
	if strings.Contains(serverName, "=") {
		serverName = ""
	} else {
		static = true
	}
	if c.config.TLS != nil && c.config.TLS.ServerName != "" {
		serverName = c.config.TLS.ServerName
	}

	config := discovery.Config{
		Addresses:   c.config.Addresses,
		GRPCPort:    c.config.GRPCPort,
		Credentials: c.config.Credentials,
	}

	if !c.config.PlainText {
		config.TLS = c.config.TLS
	}

	globalWatcherMutex.Lock()
	watcher, err := discovery.NewWatcher(ctx, config, c.config.Logger)
	globalWatcherMutex.Unlock()

	if err != nil {
		c.initialized <- err
		return err
	}
	go watcher.Run()
	defer watcher.Stop()

	// Wait for initial state.
	state, err := watcher.State()
	if err != nil {
		c.initialized <- err
		return err
	}
	updateClient := func(s discovery.State) error {
		cfg := c.config.ApiClientConfig
		cfg.Address = fmt.Sprintf("%s:%d", s.Address.IP.String(), c.config.HTTPPort)
		if static {
			// This is to fix the fact that s.Address always resolves to an IP, if
			// we pass a DNS address without an IPSANS, regardless of setting cfg.TLSConfig.Address
			// below, we have a connection error on cert validation.
			cfg.Address = fmt.Sprintf("%s:%d", c.config.Addresses, c.config.HTTPPort)
		}
		cfg.Token = s.Token
		cfg.TLSConfig.Address = serverName

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
		c.initialized <- err
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
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Agent()
}

func (c *client) Catalog() *api.Catalog {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Catalog()
}

func (c *client) ConfigEntries() *api.ConfigEntries {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.ConfigEntries()
}

func (c *client) DiscoveryChain() *api.DiscoveryChain {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.DiscoveryChain()
}

func (c *client) Namespaces() *api.Namespaces {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Namespaces()
}

func (c *client) ACL() *api.ACL {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.ACL()
}

func (c *client) Token() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.token
}

func (c *client) Internal() *api.Client {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client
}
