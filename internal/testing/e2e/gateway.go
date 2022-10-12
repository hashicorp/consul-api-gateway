package e2e

import (
	"context"
	"log"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/hashicorp/go-hclog"

	consulAdapters "github.com/hashicorp/consul-api-gateway/internal/adapters/consul"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

type gatewayTestContext struct{}

var gatewayTestContextKey = gatewayTestContext{}

type gatewayTestEnvironment struct {
	cancel             func()
	group              *errgroup.Group
	directory          string
	serviceAccountName string
}

func (p *gatewayTestEnvironment) run(ctx context.Context, namespace string, cfg *envconf.Config) error {
	serviceAccountClient, err := serviceAccountClient(ctx, cfg.Client(), p.serviceAccountName, namespace)
	if err != nil {
		return err
	}

	consulClient := ConsulClient(ctx)

	// this should go away once we implement auth in the server bootup
	consulClient.AddHeader("x-consul-token", ConsulInitialManagementToken(ctx))

	nullLogger := hclog.Default()
	nullLogger.SetLevel(hclog.Trace)
	// nullLogger := hclog.NewNullLogger()

	secretClient := envoy.NewMultiSecretClient()
	k8sSecretClient, err := k8s.NewK8sSecretClient(nullLogger, serviceAccountClient.RESTConfig())
	if err != nil {
		return err
	}

	secretClient.Register(utils.K8sSecretScheme, k8sSecretClient)

	k8sConfig := &k8s.Config{
		SDSServerHost: HostRoute(ctx),
		SDSServerPort: 9090,
		RestConfig:    serviceAccountClient.RESTConfig(),
		CACert:        ConsulCA(ctx),
		ConsulNamespaceConfig: k8s.ConsulNamespaceConfig{
			ConsulDestinationNamespace: ConsulNamespace(ctx),
			MirrorKubernetesNamespaces: IsEnterprise(),
		},
	}

	controller, err := k8s.New(nullLogger, k8sConfig)
	if err != nil {
		return err
	}

	adapter := consulAdapters.NewSyncAdapter(nullLogger, consulClient)
	store := store.New(k8s.StoreConfig(adapter, controller.Client(), consulClient, nullLogger, *k8sConfig))

	controller.SetConsul(consulClient)
	controller.SetStore(store)

	// set up the cert manager
	certManagerOptions := consul.DefaultCertManagerOptions()
	certManagerOptions.Directory = p.directory
	certManager := consul.NewCertManager(
		nullLogger,
		consulClient,
		"consul-api-gateway",
		certManagerOptions,
	)

	// wait for the first write
	cancelCtx, cancel := context.WithCancel(ctx)
	group, groupCtx := errgroup.WithContext(cancelCtx)
	group.Go(func() error {
		return certManager.Manage(groupCtx)
	})
	timeoutCtx, timeoutCancel := context.WithTimeout(groupCtx, 10*time.Second)
	defer timeoutCancel()
	err = certManager.WaitForWrite(timeoutCtx)
	if err != nil {
		cancel()
		return err
	}

	sds := envoy.NewSDSServer(nullLogger, certManager, secretClient, store)
	// boot the sds server and controller
	group.Go(func() error {
		return sds.Run(groupCtx)
	})
	group.Go(func() error {
		return controller.Start(groupCtx)
	})
	group.Go(func() error {
		store.SyncAllAtInterval(groupCtx)
		return nil
	})
	p.cancel = cancel
	p.group = group
	return nil
}

func (p *gatewayTestEnvironment) stop() error {
	p.cancel()
	return p.group.Wait()
}

func CreateTestGatewayServer(namespace string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Print("Creating Test Consul API Gateway Server")

		tmpdir, err := os.MkdirTemp("", "consul-api-gateway-e2e")
		if err != nil {
			return nil, err
		}
		env := &gatewayTestEnvironment{
			serviceAccountName: "consul-api-gateway",
			directory:          tmpdir,
		}
		if err := env.run(ctx, namespace, cfg); err != nil {
			return nil, err
		}
		return context.WithValue(ctx, gatewayTestContextKey, env), nil
	}
}

func DestroyTestGatewayServer(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Destroying Test Consul API Gateway Server")

	polarEnvironment := ctx.Value(gatewayTestContextKey)
	if polarEnvironment == nil {
		return ctx, nil
	}
	env := polarEnvironment.(*gatewayTestEnvironment)
	if err := env.stop(); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(env.directory); err != nil {
		return nil, err
	}
	return ctx, nil
}
