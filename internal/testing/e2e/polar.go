package e2e

import (
	"context"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/common"
	"github.com/hashicorp/polar/internal/consul"
	"github.com/hashicorp/polar/internal/envoy"
	"github.com/hashicorp/polar/k8s"
	polarv1alpha1 "github.com/hashicorp/polar/k8s/apis/v1alpha1"
	"golang.org/x/sync/errgroup"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type polarTestContext struct{}

var polarTestContextKey = polarTestContext{}

type polarTestEnvironment struct {
	cancel    func()
	group     *errgroup.Group
	directory string
}

func (p *polarTestEnvironment) run(ctx context.Context) error {
	consulClient := ConsulClient(ctx)

	// this should go away once we implement auth in the server bootup
	consulClient.AddHeader("x-consul-token", ConsulMasterToken(ctx))

	// nullLogger := hclog.Default()
	nullLogger := hclog.NewNullLogger()
	registry := common.NewGatewaySecretRegistry()

	// set up the cert manager
	certManagerOptions := consul.DefaultCertManagerOptions()
	certManagerOptions.Directory = p.directory
	certManager := consul.NewCertManager(
		nullLogger,
		consulClient,
		"polar-test-controller",
		certManagerOptions,
	)

	// set up the controller
	options := &k8s.Options{
		CACertSecretNamespace: "default",
		CACertSecret:          "",
		CACertFile:            "",
		SDSServerHost:         "polar-controller.default.svc.cluster.local",
		SDSServerPort:         9090,
		MetricsBindAddr:       ":8080",
		HealthProbeBindAddr:   ":8081",
		WebhookPort:           8443,
	}
	controller, err := k8s.New(nullLogger, registry, options)
	if err != nil {
		return err
	}
	controller.SetConsul(consulClient)

	// set up the SDS server
	secretClient, err := k8s.NewK8sSecretClient(nullLogger)
	if err != nil {
		return err
	}
	sds := envoy.NewSDSServer(nullLogger, certManager, secretClient, registry)

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

func (p *polarTestEnvironment) stop() error {
	p.cancel()
	return p.group.Wait()
}

func CreateTestPolarServer(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Creating Test Polar Server")

	tmpdir, err := os.MkdirTemp("", "polar-integration")
	if err != nil {
		return nil, err
	}
	env := &polarTestEnvironment{
		directory: tmpdir,
	}
	if err := env.run(ctx); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, polarTestContextKey, env), nil
}

func DestroyTestPolarServer(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Destroying Test Polar Server")

	polarEnvironment := ctx.Value(polarTestContextKey)
	if polarEnvironment == nil {
		return ctx, nil
	}
	env := polarEnvironment.(*polarTestEnvironment)
	if err := env.stop(); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(env.directory); err != nil {
		return nil, err
	}
	return ctx, nil
}

func InstallPolarCRDs(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	directory := path.Join("..", "..", "config", "crd", "bases")
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

	groupVersion := schema.GroupVersion{Group: "polar.hashicorp.com", Version: "v1alpha1"}
	scheme.Scheme.AddKnownTypes(groupVersion, &polarv1alpha1.GatewayClassConfig{}, &polarv1alpha1.GatewayClassConfigList{})
	meta.AddToGroupVersion(scheme.Scheme, groupVersion)

	return ctx, nil
}
