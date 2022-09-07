package internal

import (
	"errors"

	"github.com/deepmap/oapi-codegen/pkg/securityprovider"
)

// TODO(andrew): most of this is boilerplate that should be generated

const defaultServerBase = "/api/internal"

type ClientConfig struct {
	Server  string
	BaseURL string
	Token   string
}

type APIClient struct {
	client *ClientWithResponses
}

func CreateClient(config ClientConfig) (*APIClient, error) {
	if config.Server == "" {
		return nil, errors.New("must specify a server value")
	}
	clientOptions := []ClientOption{}

	if config.Token != "" {
		// error ignored because when the first parameter is "header" an error is never returned
		apiKeyProvider, _ := securityprovider.NewSecurityProviderApiKey("header", "X-API-Gateway-Token", config.Token)
		clientOptions = append(clientOptions, WithRequestEditorFn(apiKeyProvider.Intercept))
	}

	serverBase := defaultServerBase
	if config.BaseURL != "" {
		serverBase = config.BaseURL
	}

	client, err := NewClientWithResponses(config.Server+serverBase, clientOptions...)
	if err != nil {
		return nil, err
	}
	return &APIClient{client: client}, nil
}
