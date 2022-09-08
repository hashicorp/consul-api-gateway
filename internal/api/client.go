package api

import (
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/api/internal"
	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
)

type Client struct {
	v1       *v1.APIClient
	internal *internal.APIClient
}

type TLSConfiguration struct {
	CAFile           string
	SkipVerification bool
}

type ClientConfig struct {
	Address          string
	Port             uint
	Token            string
	GatewayToken     string
	TLSConfiguration *TLSConfiguration
}

func CreateClient(config ClientConfig) (*Client, error) {
	caFile := ""
	skipVerification := false
	scheme := "http"
	if config.TLSConfiguration != nil {
		// we're using SSL
		scheme = "https"
		caFile = config.TLSConfiguration.CAFile
		skipVerification = config.TLSConfiguration.SkipVerification
	}

	server := fmt.Sprintf("%s://%s", scheme, config.Address)
	if config.Port != 0 {
		server += fmt.Sprintf(":%d", config.Port)
	}
	v1Client, err := v1.CreateClient(v1.ClientConfig{
		Server: server,
		Token:  config.Token,
		TLSConfiguration: &v1.ClientTLSConfiguration{
			CAFile:           caFile,
			SkipVerification: skipVerification,
		},
	})
	if err != nil {
		return nil, err
	}

	internalClient, err := internal.CreateClient(internal.ClientConfig{
		Server: server,
		Token:  config.GatewayToken,
		TLSConfiguration: &internal.ClientTLSConfiguration{
			CAFile:           caFile,
			SkipVerification: skipVerification,
		},
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
