package internal

import (
	"errors"

	"github.com/deepmap/oapi-codegen/pkg/securityprovider"
)

// TODO(andrew): most of this is boilerplate that should be generated

const serverBase = "/api/internal"

type ClientConfig struct {
	Server string
	Base   string
	Token  string
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
		apiKeyProvider, _ := securityprovider.NewSecurityProviderApiKey("header", "X-Gateway-Token", config.Token)
		clientOptions = append(clientOptions, WithRequestEditorFn(apiKeyProvider.Intercept))
	}

	serverBase := serverBase
	if config.Base != "" {
		serverBase = config.Base
	}

	client, err := NewClientWithResponses(config.Server+serverBase, clientOptions...)
	if err != nil {
		return nil, err
	}
	return &APIClient{client: client}, nil
}
