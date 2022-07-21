package vault

import (
	"context"
	"time"

	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/api"
)

const SecretScheme = "vault"

type SecretClient struct {
	logger hclog.Logger
	client *api.Client
}

func NewSecretClient(logger hclog.Logger, config *api.Config) (*SecretClient, error) {
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &SecretClient{
		logger: logger,
		client: client,
	}, nil
}

func (c *SecretClient) FetchSecret(ctx context.Context, fullName string) (*tls.Secret, time.Time, error) {
	//TODO implement me
	panic("implement me")
}
