// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type PeeringClient interface {
	Read(ctx context.Context, name string, q *api.QueryOptions) (*api.Peering, *api.QueryMeta, error)
}

type Client interface {
	Agent() *api.Agent
	ACL() *api.ACL
	Catalog() *api.Catalog
	ConfigEntries() *api.ConfigEntries
	ConsulAddress() string
	DiscoveryChain() *api.DiscoveryChain
	Namespaces() *api.Namespaces
	Peerings() PeeringClient

	WatchServers(ctx context.Context) error

	Token() string
	Wait(until time.Duration) error

	// TODO: drop this
	Internal() *api.Client
}

type ClientConfig struct {
	Name            string
	Namespace       string
	ApiClientConfig *api.Config
	UseDynamic      bool
	PlainText       bool
	Addresses       string
	HTTPPort        int
	GRPCPort        int
	TLS             *tls.Config
	Credentials     discovery.Credentials
	Logger          hclog.Logger
}

type client struct {
	config        ClientConfig
	client        *api.Client
	consulAddress string
	token         string
	mutex         sync.RWMutex
	initialized   chan error
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
		return errors.New("did not get state within time limit")
	}
}

func (c *client) WatchServers(ctx context.Context) error {
	if !c.config.UseDynamic {
		cfg := c.config.ApiClientConfig
		cfg.Address = fmt.Sprintf("%s:%d", c.config.Addresses, c.config.HTTPPort)

		var err error
		var client *api.Client
		var token string
		if c.config.Credentials.Type == discovery.CredentialsTypeLogin {
			baseClient, err := api.NewClient(cfg)
			if err != nil {
				c.initialized <- err
				return err
			}
			if c.config.Namespace != "" {
				cfg.Namespace = c.config.Namespace
			}
			client, token, err = login(ctx, baseClient, cfg, c.config)
			if err != nil {
				c.initialized <- err
				return err
			}
			defer logout(baseClient, token, c.config)

		} else {
			// this might be empty
			cfg.Token = c.config.Credentials.Static.Token
			if c.config.Namespace != "" {
				cfg.Namespace = c.config.Namespace
			}
			client, err = api.NewClient(cfg)
			if err != nil {
				c.initialized <- err
				return err
			}
		}

		c.mutex.Lock()
		c.client = client
		c.token = cfg.Token
		c.mutex.Unlock()

		close(c.initialized)

		<-ctx.Done()
		return nil
	}

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

	watcher, err := discovery.NewWatcher(ctx, config, c.config.Logger)

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
		var consulAddress string

		fmt.Printf("DISCOVERY STATE %+v\n", s)

		cfg := c.config.ApiClientConfig
		if c.config.Namespace != "" {
			cfg.Namespace = c.config.Namespace
		}

		consulAddress = s.Address.IP.String()
		cfg.Address = fmt.Sprintf("%s:%d", consulAddress, c.config.HTTPPort)
		if static {
			// This is to fix the fact that s.Address always resolves to an IP, if
			// we pass a DNS address without an IPSANS, regardless of setting cfg.TLSConfig.Address
			// below, we have a connection error on cert validation.
			consulAddress = c.config.Addresses
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
		c.consulAddress = consulAddress
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

func (c *client) ConsulAddress() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.consulAddress
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

func (c *client) Peerings() PeeringClient {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.client.Peerings()
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

func login(ctx context.Context, client *api.Client, cfg *api.Config, config ClientConfig) (*api.Client, string, error) {
	authenticator := NewAuthenticator(
		config.Logger.Named("authenticator"),
		client,
		config.Credentials.Login.AuthMethod,
		config.Credentials.Login.Namespace,
	)

	token, err := authenticator.Authenticate(ctx, config.Name, config.Credentials.Login.BearerToken)
	if err != nil {
		return nil, "", fmt.Errorf("error logging in to consul: %w", err)
	}

	// Now update the client so that it will read the ACL token we just fetched.
	cfg.Token = token
	newClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("error updating client connection with token: %w", err)
	}
	return newClient, token, nil
}

func logout(client *api.Client, token string, config ClientConfig) error {
	config.Logger.Info("deleting acl token")
	_, err := client.ACL().Logout(&api.WriteOptions{Token: token})
	if err != nil {
		return fmt.Errorf("error deleting acl token: %w", err)
	}
	return nil
}
