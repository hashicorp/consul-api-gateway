package internal

import (
	"crypto/x509"
	"errors"
	"net/http"
	"os"

	"github.com/deepmap/oapi-codegen/pkg/securityprovider"
)

// TODO(andrew): most of this is boilerplate that should be generated

const defaultServerBase = "/api/internal"

type ClientTLSConfiguration struct {
	CAFile           string
	SkipVerification bool
}

type ClientConfig struct {
	Server           string
	BaseURL          string
	Token            string
	TLSConfiguration *ClientTLSConfiguration
}

type APIClient struct {
	client *ClientWithResponses
}

func CreateClient(config ClientConfig) (*APIClient, error) {
	if config.Server == "" {
		return nil, errors.New("must specify a server value")
	}
	clientOptions := []ClientOption{}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	httpClient := &http.Client{
		Transport: transport,
	}
	if config.TLSConfiguration != nil {
		transport.TLSClientConfig.InsecureSkipVerify = config.TLSConfiguration.SkipVerification
		if config.TLSConfiguration.CAFile != "" {
			pem, err := os.ReadFile(config.TLSConfiguration.CAFile)
			if err != nil {
				return nil, err
			}
			certPool := x509.NewCertPool()
			if !certPool.AppendCertsFromPEM(pem) {
				return nil, err
			}
			transport.TLSClientConfig.RootCAs = certPool
		}
	}
	clientOptions = append(clientOptions, WithHTTPClient(httpClient))

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
