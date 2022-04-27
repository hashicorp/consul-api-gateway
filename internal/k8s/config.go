package k8s

import (
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/consul-api-gateway/internal/store/memory"
	"github.com/hashicorp/go-hclog"
)

func StoreConfig(adapter core.SyncAdapter, client gatewayclient.Client, logger hclog.Logger, config Config) store.Config {
	marshaler := reconciler.NewMarshaler()
	binder := reconciler.NewBinder(client)
	deployer := reconciler.NewDeployer(reconciler.DeployerConfig{
		ConsulCA: config.CACert,
		SDSHost:  config.SDSServerHost,
		SDSPort:  config.SDSServerPort,
		Logger:   logger,
		Client:   client,
	})
	updater := reconciler.NewStatusUpdater(logger, client, deployer, ControllerName)
	backend := memory.NewBackend()

	return store.Config{
		Logger:        logger,
		Adapter:       adapter,
		Backend:       backend,
		Marshaler:     marshaler,
		StatusUpdater: updater,
		Binder:        binder,
	}
}
