package vm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/require"
)

func Test_TestController(t *testing.T) {
	controller := TestController(t)

	_, err := controller.Client.V1().Health(context.Background())
	require.NoError(t, err)

	name := "stub-deployment" // this should come from a created gateway.Name attribute
	token := "token"          // this should come from token minting
	controller.Deployment(t, name, token)

	// check gateway registration
	services, _, err := controller.Consul.Client.Catalog().Service(name, "", nil)
	require.NoError(t, err)
	require.Len(t, services, 1)

	// check the target registration routines
	target := controller.RegisterHTTPServiceTarget(t)
	require.NoError(t, backoff.Retry(func() error {
		services, _, err := controller.Consul.Client.Catalog().Service(target.Name, "", nil)
		if err != nil {
			return err
		}
		if len(services) != 1 {
			return errors.New("proxy target not found")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 10)))

	target = controller.RegisterTCPServiceTarget(t)
	require.NoError(t, backoff.Retry(func() error {
		services, _, err := controller.Consul.Client.Catalog().Service(target.Name, "", nil)
		if err != nil {
			return err
		}
		if len(services) != 1 {
			return errors.New("proxy target not found")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 10)))
}
