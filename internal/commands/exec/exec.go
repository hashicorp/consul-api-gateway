package exec

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
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
	Context       context.Context
	Logger        hclog.Logger
	LogLevel      string
	ConsulClient  *api.Client
	ConsulConfig  api.Config
	AuthConfig    AuthConfig
	GatewayConfig GatewayConfig
	EnvoyConfig   EnvoyConfig

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

	// First do the ACL Login, if necessary.
	consulClient := config.ConsulClient
	var token string
	var err error
	if config.AuthConfig.Method != "" {
		config.Logger.Trace("logging in to consul")
		consulClient, token, err = login(config)
		if err != nil {
			config.Logger.Error("error logging into consul", "error", err)
			return 1
		}
		config.Logger.Trace("consul login complete")
	}

	registry := consul.NewServiceRegistry(
		config.Logger.Named("service-registry"),
		consulClient,
		config.GatewayConfig.Name,
		config.GatewayConfig.Namespace,
		config.GatewayConfig.Host,
	)
	if config.isTest {
		registry = registry.WithTries(1)
	}

	config.Logger.Trace("registering service")
	if err := registry.Register(ctx); err != nil {
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
			ConsulCA:          config.EnvoyConfig.CACertificateFile,
			ConsulAddress:     config.EnvoyConfig.XDSAddress,
			ConsulXDSPort:     config.EnvoyConfig.XDSPort,
			BootstrapFilePath: config.EnvoyConfig.BootstrapFile,
			LogLevel:          config.LogLevel,
			Token:             token,
			EnvoyBinary:       config.EnvoyConfig.Binary,
			ExtraArgs:         config.EnvoyConfig.ExtraArgs,
			Output:            config.EnvoyConfig.Output,
		},
	)
	options := consul.DefaultCertManagerOptions()
	options.SDSAddress = config.EnvoyConfig.SDSAddress
	options.SDSPort = config.EnvoyConfig.SDSPort
	options.Directory = "/certs"
	if config.EnvoyConfig.CertificateDirectory != "" {
		options.Directory = config.EnvoyConfig.CertificateDirectory
	}
	certManager := consul.NewCertManager(
		config.Logger.Named("cert-manager"),
		consulClient,
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

func login(config ExecConfig) (*api.Client, string, error) {
	authenticator := consul.NewAuthenticator(
		config.Logger.Named("authenticator"),
		config.ConsulClient,
		config.AuthConfig.Method,
		config.AuthConfig.Namespace,
	)
	if config.isTest {
		authenticator = authenticator.WithTries(1)
	}

	token, err := authenticator.Authenticate(config.Context, config.GatewayConfig.Name, config.AuthConfig.Token)
	if err != nil {
		return nil, "", fmt.Errorf("error logging in to consul: %w", err)
	}

	// Now update the client so that it will read the ACL token we just fetched.
	config.ConsulConfig.Token = token
	newClient, err := api.NewClient(&config.ConsulConfig)
	if err != nil {
		return nil, "", fmt.Errorf("error updating client connection with token: %w", err)
	}
	return newClient, token, nil
}
