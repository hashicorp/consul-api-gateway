package vm

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/uuid"
	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

type Vault struct {
	Client *api.Client
	Config *api.Config
	Token  string
}

func TestVault(t *testing.T) *Vault {
	t.Helper()

	vault, err := exec.LookPath("vault")
	require.NoError(t, err)

	token := uuid.New().String()

	port := freeport.MustTake(1)[0]
	address := fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, vault, "server", "-dev", "-dev-listen-address", address, "-dev-no-store-token", "-dev-no-store-token", "-dev-root-token-id", token)
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		cmd.Process.Kill()
		cancel()
	})

	config := &api.Config{
		Address: "http://" + address,
	}
	client, err := api.NewClient(config)
	require.NoError(t, err)
	client.SetToken(token)

	require.NoError(t, backoff.Retry(func() error {
		_, err := client.Sys().Health()
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 10)))

	return &Vault{
		Client: client,
		Token:  token,
		Config: config,
	}
}
