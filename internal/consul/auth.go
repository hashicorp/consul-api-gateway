package consul

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

const (
	authMetaKey = "consul-api-gateway"
)

// Authenticator handles Consul auth login logic.
type Authenticator struct {
	consul *api.Client
	logger hclog.Logger

	method          string
	namespace       string
	tries           uint64
	backoffInterval time.Duration
}

// NewAuthenticator initializes a new Authenticator instance.
func NewAuthenticator(logger hclog.Logger, consul *api.Client, method, namespace string) *Authenticator {
	return &Authenticator{
		consul:          consul,
		logger:          logger,
		method:          method,
		namespace:       namespace,
		tries:           defaultMaxAttempts,
		backoffInterval: defaultBackoffInterval,
	}
}

func (a *Authenticator) WithTries(tries uint64) *Authenticator {
	a.tries = tries
	return a
}

// Authenticate logs into Consul using the given auth method and returns the generated
// token.
func (a *Authenticator) Authenticate(ctx context.Context, service, bearerToken string) (string, error) {
	var token string
	var err error

	err = backoff.Retry(func() error {
		token, err = a.authenticate(ctx, service, bearerToken)
		if err != nil {
			a.logger.Error("error authenticating", "error", err)
		}
		return err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(a.backoffInterval), a.tries), ctx))
	return token, err
}

func (a *Authenticator) authenticate(ctx context.Context, service, bearerToken string) (string, error) {
	gwName := service

	opts := &api.WriteOptions{}
	if a.namespace != "" && a.namespace != "default" {
		opts.Namespace = a.namespace
		gwName = fmt.Sprintf("%s/%s", a.namespace, service)
	}

	token, _, err := a.consul.ACL().Login(&api.ACLLoginParams{
		AuthMethod:  a.method,
		BearerToken: bearerToken,
		Meta:        map[string]string{authMetaKey: gwName},
	}, opts.WithContext(ctx))
	if err != nil {
		return "", err
	}
	return token.SecretID, nil
}
