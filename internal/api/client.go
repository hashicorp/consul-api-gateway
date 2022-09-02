package api

import (
	"github.com/hashicorp/consul-api-gateway/internal/api/internal"
	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
)

// TODO(andrew) should this be generated too?

type Client struct {
	v1       *v1.APIClient
	internal *internal.APIClient
}

type ClientConfig struct {
	Server       string
	Token        string
	GatewayToken string
}

func CreateClient(config ClientConfig) (*Client, error) {
	v1Client, err := v1.CreateClient(v1.ClientConfig{
		Server: config.Server,
		Token:  config.Token,
	})
	if err != nil {
		return nil, err
	}
	internalClient, err := internal.CreateClient(internal.ClientConfig{
		Server: config.Server,
		Token:  config.GatewayToken,
	})
	if err != nil {
		return nil, err
	}
	return &Client{
		v1:       v1Client,
		internal: internalClient,
	}, nil
}

func (c *Client) V1() *v1.APIClient {
	return c.v1
}

func (c *Client) Internal() *internal.APIClient {
	return c.internal
}
