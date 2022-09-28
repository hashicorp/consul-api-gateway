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
	"github.com/google/uuid"
	"github.com/hashicorp/consul-api-gateway/internal/api"
	"github.com/hashicorp/consul-api-gateway/internal/commands/controller"
	"github.com/hashicorp/consul-api-gateway/internal/commands/deployment"
	consulapi "github.com/hashicorp/consul/api"
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

	ctx context.Context
}

func TestController(t *testing.T) *Controller {
	t.Helper()

	consul := TestConsul(t, true)
	vault := TestVault(t)
	return runController(t, context.Background(), vault, consul)
}

func (c *Controller) PeerController(t *testing.T) *Controller {
	t.Helper()

	return runController(t, c.ctx, c.Vault, c.Consul)
}

func runController(t *testing.T, ctx context.Context, vault *Vault, consul *Consul) *Controller {
	t.Helper()

	ctx, cancel := context.WithCancel(ctx)
	command := controller.NewCommand(ctx, cli.NewMockUi(), io.Discard)

	ports := freeport.MustTake(4)
	apiPort := ports[0]
	sdsPort := ports[1]
	metricsPort := ports[2]
	profPort := ports[3]

	go func() {
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
		ctx:    ctx,
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

func (c *Controller) RegisterHTTPServiceTarget(t *testing.T) *ProxyTarget {
	t.Helper()

	return c.registerServiceTargetWithName(t, "http", uuid.New().String())
}

func (c *Controller) RegisterHTTPServiceTargetWithName(t *testing.T, name string) *ProxyTarget {
	t.Helper()

	return c.registerServiceTargetWithName(t, "http", name)
}

func (c *Controller) RegisterTCPServiceTarget(t *testing.T) *ProxyTarget {
	t.Helper()

	return c.registerServiceTargetWithName(t, "tcp", uuid.New().String())
}

func (c *Controller) RegisterTCPServiceTargetWithName(t *testing.T, name string) *ProxyTarget {
	t.Helper()

	return c.registerServiceTargetWithName(t, "tcp", name)
}

func (c *Controller) registerServiceTargetWithName(t *testing.T, protocol, name string) *ProxyTarget {
	t.Helper()

	template := httpBootstrapTemplate
	if protocol == "tcp" {
		template = tcpBootstrapTemplate
	}

	target := c.runProxyTarget(t, name, template)

	client := c.Consul.Client

	registration := &consulapi.AgentServiceRegistration{
		ID:      target.Name,
		Name:    target.Name,
		Port:    target.Port,
		Address: "127.0.0.1",
	}
	require.NoError(t, client.Agent().ServiceRegisterOpts(registration, consulapi.ServiceRegisterOpts{}))

	_, _, err := client.ConfigEntries().Set(&consulapi.ServiceConfigEntry{
		Kind:     consulapi.ServiceDefaults,
		Name:     target.Name,
		Protocol: protocol,
	}, nil)
	require.NoError(t, err)

	proxyRegistration := &consulapi.AgentServiceRegistration{
		Kind: consulapi.ServiceKindConnectProxy,
		ID:   target.Name,
		Name: target.Name,
		Port: target.ProxyPort,
		Proxy: &consulapi.AgentServiceConnectProxyConfig{
			DestinationServiceName: target.Name,
			LocalServiceAddress:    "127.0.0.1",
			LocalServicePort:       target.Port,
		},
		Address: "127.0.0.1",
	}
	require.NoError(t, client.Agent().ServiceRegisterOpts(proxyRegistration, consulapi.ServiceRegisterOpts{}))

	return target
}
