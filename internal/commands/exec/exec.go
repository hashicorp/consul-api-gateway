package exec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul-server-connection-manager/discovery"
)

type AuthConfig struct {
	Method    string
	Namespace string
	Token     string
}

type GatewayConfig struct {
	Host      string
	Name      string
	Namespace string
}

type EnvoyConfig struct {
	CACertificateFile    string
	XDSAddress           string
	XDSPort              int
	SDSAddress           string
	SDSPort              int
	BootstrapFile        string
	CertificateDirectory string
	Binary               string
	ExtraArgs            []string
	Output               io.Writer
}

type ExecConfig struct {
	Context           context.Context
	Logger            hclog.Logger
	LogLevel          string
	ConsulClient      *api.Client
	ConsulConfig      api.Config
	ConsulHTTPAddress string
	ConsulHTTPPort    int
	ConsulGRPCPort    int
	AuthConfig        AuthConfig
	GatewayConfig     GatewayConfig
	EnvoyConfig       EnvoyConfig
	PrimaryDatacenter string

	// for testing only
	isTest bool
}

func RunExec(config ExecConfig) (ret int) {
	// Set up signal handlers and global context
	ctx, cancel := context.WithCancel(config.Context)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(interrupt)
		cancel()
	}()
	go func() {
		select {
		case <-interrupt:
			config.Logger.Debug("received shutdown signal")
			cancel()
		case <-ctx.Done():
		}
	}()

	// ConsulServerConnMgr is the watcher for the Consul server addresses.
	wctx, wcancel := context.WithTimeout(ctx, 3*time.Second)

	// Release resources if consulServerConnMgr completes before timeout elapses
	defer wcancel()

	discoveryConfig := discovery.Config{
		Addresses: config.ConsulHTTPAddress,
		GRPCPort:  config.ConsulGRPCPort,
		Credentials: discovery.Credentials{
			Type: discovery.CredentialsTypeLogin,
			Login: discovery.LoginCredential{
				AuthMethod:  config.AuthConfig.Method,
				Namespace:   config.AuthConfig.Method,
				BearerToken: config.AuthConfig.Token,
			},
		},
	}
	if config.isTest {
		discoveryConfig.ServerWatchDisabled = true
	}

	consulServerConnMgr, err := discovery.NewWatcher(wctx, discoveryConfig, config.Logger)
	if err != nil {
		config.Logger.Error("failed to start Consul server connection manager", err)
		return 1
	}

	// Start Consul server discovery connection manager watcher
	go consulServerConnMgr.Run()
	defer consulServerConnMgr.Stop()

	// Wait for initial state.
	serverState, err := consulServerConnMgr.State()
	if err != nil {
		config.Logger.Error("failed to get Consul server state", err)
		return 1
	}
	config.Logger.Trace("%#v", serverState)

	consulClient, err := makeClient(config, serverState)
	if err != nil {
		config.Logger.Error("failed to get Consul server state", err)
		return 1
	}
	client := consul.NewClient(ctx, consulClient)
	registry := consul.NewServiceRegistry(
		config.Logger.Named("service-registry"),
		client,
		config.GatewayConfig.Name,
		config.GatewayConfig.Namespace,
		config.GatewayConfig.Host,
	)
	if config.isTest {
		registry = registry.WithTries(1)
	}

	config.Logger.Trace("registering service")
	if err := registry.RegisterGateway(ctx, true); err != nil {
		config.Logger.Error("error registering service", "error", err)
		return 1
	}
	defer func() {
		config.Logger.Trace("deregistering service")
		// using context.Background here since the global context has
		// already been canceled at this point and we're just in a cleanup
		// function
		if err := registry.Deregister(context.Background()); err != nil {
			config.Logger.Error("error deregistering service", "error", err)
			ret = 1
		}
	}()

	envoyManager := envoy.NewManager(
		config.Logger.Named("envoy-manager"),
		envoy.ManagerConfig{
			ID:                registry.ID(),
			Namespace:         registry.Namespace(),
			ConsulCA:          config.EnvoyConfig.CACertificateFile,
			ConsulAddress:     config.EnvoyConfig.XDSAddress,
			ConsulXDSPort:     config.EnvoyConfig.XDSPort,
			BootstrapFilePath: config.EnvoyConfig.BootstrapFile,
			LogLevel:          config.LogLevel,
			Token:             config.ConsulConfig.Token,
			EnvoyBinary:       config.EnvoyConfig.Binary,
			ExtraArgs:         config.EnvoyConfig.ExtraArgs,
			Output:            config.EnvoyConfig.Output,
		},
	)
	options := consul.DefaultCertManagerOptions()
	options.PrimaryDatacenter = config.PrimaryDatacenter
	options.SDSAddress = config.EnvoyConfig.SDSAddress
	options.SDSPort = config.EnvoyConfig.SDSPort
	options.Directory = "/certs"
	if config.EnvoyConfig.CertificateDirectory != "" {
		options.Directory = config.EnvoyConfig.CertificateDirectory
	}
	certManager := consul.NewCertManager(
		config.Logger.Named("cert-manager"),
		client,
		config.GatewayConfig.Name,
		options,
	)
	sdsConfig, err := certManager.RenderSDSConfig()
	if err != nil {
		config.Logger.Error("error rendering SDS configuration files", "error", err)
		return 1
	}
	err = envoyManager.RenderBootstrap(sdsConfig)
	if err != nil {
		config.Logger.Error("error rendering Envoy configuration file", "error", err)
		return 1
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return certManager.Manage(groupCtx)
	})

	// wait until we've written once before booting envoy
	waitTime := defaultCertWaitTime
	if config.isTest {
		waitTime = 100 * time.Millisecond
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, waitTime)
	defer waitCancel()
	config.Logger.Trace("waiting for initial certs to be written")
	if err := certManager.WaitForWrite(waitCtx); err != nil {
		config.Logger.Error("timeout waiting for certs to be written", "error", err)
		return 1
	}
	config.Logger.Trace("initial certificates written")

	group.Go(func() error {
		return envoyManager.Run(ctx)
	})

	config.Logger.Info("started consul-api-gateway api gateway")
	if err := group.Wait(); err != nil {
		config.Logger.Error("unexpected error", "error", err)
		return 1
	}

	config.Logger.Info("shutting down")
	return 0
}

func makeClient(config ExecConfig, s discovery.State) (*api.Client, error) {
	config.ConsulConfig.Address = fmt.Sprintf("%s:%d", s.Address.IP.String(), config.ConsulHTTPPort)
	if s.Token != "" {
		config.ConsulConfig.Token = s.Token
	}

	if config.ConsulConfig.Transport == nil {
		tlsClientConfig, err := api.SetupTLSConfig(&config.ConsulConfig.TLSConfig)

		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS transport: %w", err)
		}

		config.ConsulConfig.Transport = &http.Transport{TLSClientConfig: tlsClientConfig}
	} else if config.ConsulConfig.Transport.TLSClientConfig == nil {
		tlsClientConfig, err := api.SetupTLSConfig(&config.ConsulConfig.TLSConfig)

		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS transport: %w", err)
		}

		config.ConsulConfig.Transport.TLSClientConfig = tlsClientConfig
	}
	config.ConsulConfig.HttpClient.Transport = config.ConsulConfig.Transport

	return api.NewClient(&config.ConsulConfig)
}
