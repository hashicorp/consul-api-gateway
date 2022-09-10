package vm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/api"
	"github.com/hashicorp/consul-api-gateway/internal/commands/controller"
	"github.com/hashicorp/consul-api-gateway/internal/commands/deployment"
	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Controller struct {
	Consul *Consul
	Vault  *Vault

	Client *api.Client
	Port   int
}

func TestController(t *testing.T) *Controller {
	t.Helper()

	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	command := controller.NewCommand(ctx, cli.NewMockUi(), io.Discard)

	ports := freeport.MustTake(4)
	apiPort := ports[0]
	sdsPort := ports[1]
	metricsPort := ports[2]
	profPort := ports[3]

	consul := TestConsul(t, true)
	vault := TestVault(t)

	wg.Add(1)
	go func() {
		defer wg.Done()

		_ = command.Run([]string{
			"-gateway-controller-port", strconv.Itoa(apiPort),
			"-sds-port", strconv.Itoa(sdsPort),
			"-debug-metrics-port", strconv.Itoa(metricsPort),
			"-debug-pprof-port", strconv.Itoa(profPort),
			"-consul-address", consul.Config.Address,
			"-consul-xds-port", strconv.Itoa(consul.XDSPort),
			"-consul-token", consul.Token,
			"-vault-address", vault.Config.Address,
			"-vault-token", vault.Token,
			"-vault-mount", "secret",
		})
	}()

	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	client, err := api.CreateClient(api.ClientConfig{
		Address: "127.0.0.1",
		Port:    uint(apiPort),
		Token:   consul.Token,
	})
	require.NoError(t, err)

	require.NoError(t, backoff.Retry(func() error {
		_, err := client.V1().Health(context.Background())
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 10)))

	return &Controller{
		Consul: consul,
		Vault:  vault,
		Client: client,
		Port:   apiPort,
	}
}

func (c *Controller) Deployment(t *testing.T, name, token string) {
	t.Helper()

	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	command := deployment.NewCommand(ctx, cli.NewMockUi(), io.Discard)

	wg.Add(1)
	go func() {
		defer wg.Done()

		_ = command.Run([]string{
			"-gateway-ip", "127.0.0.1",
			"-gateway-controller-token", token,
			"-gateway-controller-port", strconv.Itoa(c.Port),
		})
	}()

	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	require.NoError(t, backoff.Retry(func() error {
		services, _, err := c.Consul.Client.Catalog().Service(name, "", nil)
		if err != nil {
			return err
		}
		if len(services) == 0 {
			return errors.New("service not found")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 10)))
}

type CLITest struct {
	Command     func(ctx context.Context, ui cli.Ui, logOutput io.Writer) cli.Command
	ExitStatus  int
	Args        []string
	OutputCheck func(t *testing.T, output string)
	Timeout     time.Duration
}

func (c *Controller) RunCLI(t *testing.T, tt CLITest) {
	t.Helper()

	timeout := tt.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var buffer bytes.Buffer
	command := tt.Command(ctx, &cli.BasicUi{Writer: &buffer, ErrorWriter: &buffer}, &buffer)
	assert.Equal(t, tt.ExitStatus, command.Run(append([]string{
		"-log-level", "error",
		"-consul-token", c.Consul.Token,
		"-gateway-controller-port", strconv.Itoa(c.Port),
	}, tt.Args...)))

	if tt.OutputCheck != nil {
		tt.OutputCheck(t, buffer.String())
	}
}
