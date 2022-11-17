package k8s

import (
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

func StoreConfig(adapter core.SyncAdapter, client gatewayclient.Client, consulClient consul.Client, logger hclog.Logger, config Config) store.Config {
	marshaler := reconciler.NewMarshaler()
	binder := reconciler.NewBinder(client)
	deployer := reconciler.NewDeployer(reconciler.DeployerConfig{
		ConsulCA:                 config.CACert,
		SDSHost:                  config.SDSServerHost,
		SDSPort:                  config.SDSServerPort,
		Logger:                   logger,
		Client:                   client,
		Consul:                   consulClient,
		ConsulNamespaceMirroring: config.ConsulNamespaceConfig.MirrorKubernetesNamespaces,
	})
	updater := reconciler.NewStatusUpdater(logger, client, deployer, ControllerName)
	backend := store.NewMemoryBackend()

	return store.Config{
		Logger:        logger,
		Adapter:       adapter,
		Backend:       backend,
		Marshaler:     marshaler,
		StatusUpdater: updater,
		Binder:        binder,
	}
}
