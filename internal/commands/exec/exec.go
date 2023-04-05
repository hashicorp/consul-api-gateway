// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
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
	Partition string
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
	Context            context.Context
	Logger             hclog.Logger
	LogLevel           string
	ConsulClient       *api.Client
	ConsulConfig       api.Config
	AuthConfig         AuthConfig
	GatewayConfig      GatewayConfig
	EnvoyConfig        EnvoyConfig
	ConsulClientConfig consul.ClientConfig
	PrimaryDatacenter  string

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

	group, groupCtx := errgroup.WithContext(ctx)

	sessionContext, sessionCancel := context.WithCancel(context.Background())
	defer sessionCancel()

	client := consul.NewClient(config.ConsulClientConfig)
	go func() {
		if err := client.WatchServers(sessionContext); err != nil {
			config.Logger.Error("unexpected error watching servers", "error", err)
			cancel()
		}
	}()
	if err := client.Wait(10 * time.Second); err != nil {
		config.Logger.Error("unexpected error watching servers", "error", err)
		return 1
	}

	registry := consul.NewServiceRegistry(
		config.Logger.Named("service-registry"),
		client,
		config.GatewayConfig.Name,
		config.GatewayConfig.Namespace,
		config.GatewayConfig.Partition,
		config.GatewayConfig.Host,
	)
	if config.isTest {
		registry = registry.WithRetries(1)
	}

	config.Logger.Trace("registering service")
	if err := registry.RegisterGateway(ctx, true); err != nil {
		config.Logger.Error("error registering service", "error", err)
		return 1
	}
	defer func() {
		config.Logger.Trace("deregistering service")
		// using sessionContext here since the global context has
		// already been canceled at this point and we're just in a cleanup
		// function
		if err := registry.Deregister(sessionContext); err != nil {
			config.Logger.Error("error deregistering service", "error", err)
			ret = 1
		}
		sessionCancel()
	}()

	envoyManager := envoy.NewManager(
		config.Logger.Named("envoy-manager"),
		envoy.ManagerConfig{
			ID:                registry.ID(),
			Namespace:         registry.Namespace(),
			Partition:         registry.Partition(),
			ConsulCA:          config.EnvoyConfig.CACertificateFile,
			ConsulAddress:     config.EnvoyConfig.XDSAddress,
			ConsulXDSPort:     config.EnvoyConfig.XDSPort,
			BootstrapFilePath: config.EnvoyConfig.BootstrapFile,
			LogLevel:          config.LogLevel,
			Token:             client.Token(),
			EnvoyBinary:       config.EnvoyConfig.Binary,
			ExtraArgs:         config.EnvoyConfig.ExtraArgs,
			Output:            config.EnvoyConfig.Output,
			ForceTLS:          os.Getenv(api.HTTPSSLEnvName) == "true",
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
