package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	consulAdapters "github.com/hashicorp/consul-api-gateway/internal/adapters/consul"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
	"github.com/hashicorp/consul-api-gateway/internal/profiling"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/consul-api-gateway/internal/vault"
)

type ServerConfig struct {
	Context           context.Context
	Logger            hclog.Logger
	ConsulConfig      *api.Config
	K8sConfig         *k8s.Config
	ProfilingPort     int
	MetricsPort       int
	PrimaryDatacenter string

	// for testing only
	isTest bool
}

func RunServer(config ServerConfig) int {
	// Set up signal handlers and global context
	ctx, cancel := signal.NotifyContext(config.Context, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	group, groupCtx := errgroup.WithContext(ctx)

	secretClient, err := registerSecretClients(config)
	if err != nil {
		return 1
	}

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

	adapter := consulAdapters.NewSyncAdapter(config.Logger.Named("consul-adapter"), consulClient)
	store := store.New(k8s.StoreConfig(adapter, controller.Client(), config.Logger, *config.K8sConfig))

	group.Go(func() error {
		store.SyncAllAtInterval(groupCtx)
		return nil
	})

	controller.SetConsul(consulClient)
	controller.SetStore(store)

	options := consul.DefaultCertManagerOptions()
	options.PrimaryDatacenter = config.PrimaryDatacenter

	certManager := consul.NewCertManager(
		config.Logger.Named("cert-manager"),
		config.ConsulConfig.Address,
		*config.ConsulConfig,
		"consul-api-gateway-controller",
		options,
	)
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

	// Run SDS server
	server := envoy.NewSDSServer(config.Logger.Named("sds-server"), certManager, secretClient, store)
	group.Go(func() error {
		return server.Run(groupCtx)
	})

	// Run controller
	group.Go(func() error {
		return controller.Start(groupCtx)
	})

	// Run metrics server if configured
	if config.MetricsPort != 0 {
		group.Go(func() error {
			return metrics.RunServer(groupCtx, config.Logger.Named("metrics"), fmt.Sprintf("127.0.0.1:%d", config.MetricsPort))
		})
	}

	// Run profiling server if configured
	if config.ProfilingPort != 0 {
		group.Go(func() error {
			return profiling.RunServer(groupCtx, config.Logger.Named("pprof"), fmt.Sprintf("127.0.0.1:%d", config.ProfilingPort))
		})
	}

	// Wait for any of the above to exit
	if err := group.Wait(); err != nil {
		config.Logger.Error("unexpected error", "error", err)
		return 1
	}

	config.Logger.Info("shutting down")
	return 0
}

func registerSecretClients(config ServerConfig) (*envoy.MultiSecretClient, error) {
	secretClient := envoy.NewMultiSecretClient()

	k8sSecretClient, err := k8s.NewK8sSecretClient(config.Logger.Named("k8s-cert-fetcher"), config.K8sConfig.RestConfig)
	if err != nil {
		config.Logger.Error("error initializing the kubernetes secret fetcher", "error", err)
		return nil, err
	}
	secretClient.Register(utils.K8sSecretScheme, k8sSecretClient)

	vaultPKIClient, err := vault.NewPKISecretClient(config.Logger.Named("vault-pki-cert-fetcher"), "pki", "TODO")
	if err != nil {
		config.Logger.Error("error initializing the Vault PKI cert fetcher", "error", err)
		return nil, err
	}
	secretClient.Register(vault.PKISecretScheme, vaultPKIClient)

	vaultStaticClient, err := vault.NewKVSecretClient(config.Logger.Named("vault-kv-cert-fetcher"), "secret")
	if err != nil {
		config.Logger.Error("error initializing the Vault KV cert fetcher", "error", err)
		return nil, err
	}
	secretClient.Register(vault.KVSecretScheme, vaultStaticClient)

	return secretClient, nil
}
