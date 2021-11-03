package e2e

import (
	"context"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	consulAdapters "github.com/hashicorp/consul-api-gateway/internal/adapters/consul"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/store/memory"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

type gatewayTestContext struct{}

var gatewayTestContextKey = gatewayTestContext{}

type gatewayTestEnvironment struct {
	cancel    func()
	group     *errgroup.Group
	directory string
}

func (p *gatewayTestEnvironment) run(ctx context.Context, namespace string, cfg *envconf.Config) error {
	consulClient := ConsulClient(ctx)

	// this should go away once we implement auth in the server bootup
	consulClient.AddHeader("x-consul-token", ConsulMasterToken(ctx))

	nullLogger := hclog.Default()
	nullLogger.SetLevel(hclog.Trace)
	// nullLogger := hclog.NewNullLogger()

	secretClient := envoy.NewMultiSecretClient()
	k8sSecretClient, err := k8s.NewK8sSecretClient(nullLogger, cfg.Client().RESTConfig())
	if err != nil {
		return err
	}
	k8sSecretClient.AddToMultiClient(secretClient)

	controller, err := k8s.New(nullLogger, &k8s.Config{
		CACertSecretNamespace: namespace,
		CACertSecret:          "consul-ca-cert",
		SDSServerHost:         HostRoute(ctx),
		SDSServerPort:         9090,
		RestConfig:            cfg.Client().RESTConfig(),
	})
	if err != nil {
		return err
	}

	store := memory.NewStore(memory.StoreConfig{
		Adapter: consulAdapters.NewConsulSyncAdapter(nullLogger, consulClient),
		Logger:  nullLogger,
	})

	controller.SetConsul(consulClient)
	controller.SetStore(store)

	// set up the cert manager
	certManagerOptions := consul.DefaultCertManagerOptions()
	certManagerOptions.Directory = p.directory
	certManager := consul.NewCertManager(
		nullLogger,
		consulClient,
		"consul-api-gateway-controller-test",
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
			directory: tmpdir,
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

func InstallConsulAPIGatewayCRDs(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	directory := path.Join("..", "..", "..", "config", "crd", "bases")
	entries, err := os.ReadDir(directory)
	crds := []client.Object{}
	if err != nil {
		return nil, err
	}
	for _, file := range entries {
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(path.Join(directory, file.Name()))
		if err != nil {
			return nil, err
		}
		fileCRDs, err := readCRDs(data)
		if err != nil {
			return nil, err
		}
		crds = append(crds, fileCRDs...)
	}
	if _, err := envtest.InstallCRDs(cfg.Client().RESTConfig(), envtest.CRDInstallOptions{
		CRDs: crds,
	}); err != nil {
		return nil, err
	}

	apigwv1alpha1.RegisterTypes(scheme.Scheme)

	return ctx, nil
}
