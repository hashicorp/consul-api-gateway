package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	consulAdapters "github.com/hashicorp/consul-api-gateway/internal/adapters/consul"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul-api-gateway/internal/profiling"
	"github.com/hashicorp/consul-api-gateway/internal/store/memory"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type ServerConfig struct {
	Context       context.Context
	Logger        hclog.Logger
	ConsulConfig  *api.Config
	K8sConfig     *k8s.Config
	ProfilingPort int
	MetricsPort   int

	// for testing only
	isTest bool
}

func RunServer(config ServerConfig) int {
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

	secretClient := envoy.NewMultiSecretClient()
	k8sSecretClient, err := k8s.NewK8sSecretClient(config.Logger.Named("cert-fetcher"), config.K8sConfig.RestConfig)
	if err != nil {
		config.Logger.Error("error initializing the kubernetes secret fetcher", "error", err)
		return 1
	}
	k8sSecretClient.AddToMultiClient(secretClient)

	controller, err := k8s.New(config.Logger, config.K8sConfig)
	if err != nil {
		config.Logger.Error("error creating the kubernetes controller", "error", err)
		return 1
	}

	consulClient, err := api.NewClient(config.ConsulConfig)
	if err != nil {
		config.Logger.Error("error creating a Consul API client", "error", err)
		return 1
	}

	store := memory.NewStore(memory.StoreConfig{
		Adapter: consulAdapters.NewConsulSyncAdapter(config.Logger.Named("consul-adapter"), consulClient),
		Logger:  config.Logger.Named("state"),
	})

	controller.SetConsul(consulClient)
	controller.SetStore(store)

	options := consul.DefaultCertManagerOptions()
	certManager := consul.NewCertManager(
		config.Logger.Named("cert-manager"),
		consulClient,
		"consul-api-gateway-controller",
		options,
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return certManager.Manage(groupCtx)
	})

	// wait until we've written once before booting envoy
	waitCtx, waitCancel := context.WithTimeout(ctx, defaultCertWaitTime)
	defer waitCancel()
	config.Logger.Trace("waiting for initial certs to be written")
	if err := certManager.WaitForWrite(waitCtx); err != nil {
		config.Logger.Error("timeout waiting for certs to be written", "error", err)
		return 1
	}
	config.Logger.Trace("initial certificates written")

	server := envoy.NewSDSServer(config.Logger.Named("sds-server"), certManager, secretClient, store)
	group.Go(func() error {
		return server.Run(groupCtx)
	})
	group.Go(func() error {
		return controller.Start(groupCtx)
	})

	if config.MetricsPort != 0 {
		group.Go(func() error {
			return metrics.RunServer(groupCtx, config.Logger.Named("metrics"), fmt.Sprintf("127.0.0.1:%d", config.MetricsPort))
		})
	}

	if config.ProfilingPort != 0 {
		group.Go(func() error {
			return profiling.RunServer(groupCtx, config.Logger.Named("pprof"), fmt.Sprintf("127.0.0.1:%d", config.ProfilingPort))
		})
	}

	if err := group.Wait(); err != nil {
		config.Logger.Error("unexpected error", "error", err)
		return 1
	}

	config.Logger.Info("shutting down")
	return 0
}
