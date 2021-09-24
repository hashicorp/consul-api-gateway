package k8s

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	klogv2 "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	clientruntime "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/k8s/controllers"
	"github.com/hashicorp/consul-api-gateway/k8s/log"
	"github.com/hashicorp/consul-api-gateway/k8s/reconciler"
	"github.com/hashicorp/consul-api-gateway/k8s/utils"
)

var (
	scheme = runtime.NewScheme()
)

const (
	ControllerName             = "hashicorp.com/consul-api-gateway-controller"
	controllerLeaderElectionID = "consul-api-gateway.consul.hashicorp.com"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gw.AddToScheme(scheme))
}

type Kubernetes struct {
	sDSServerHost string
	sDSServerPort int
	k8sManager    ctrl.Manager
	consul        *api.Client
	registry      *common.GatewaySecretRegistry
	logger        hclog.Logger
}

type Options struct {
	CACertSecretNamespace string
	CACertSecret          string
	CACertFile            string
	SDSServerHost         string
	SDSServerPort         int
	MetricsBindAddr       string
	HealthProbeBindAddr   string
	WebhookPort           int
}

func Defaults() *Options {
	return &Options{
		CACertSecretNamespace: "default",
		CACertSecret:          "",
		CACertFile:            "",
		SDSServerHost:         "consul-api-gateway-controller.default.svc.cluster.local",
		SDSServerPort:         9090,
		MetricsBindAddr:       ":8080",
		HealthProbeBindAddr:   ":8081",
		WebhookPort:           8443,
	}
}

func New(logger hclog.Logger, registry *common.GatewaySecretRegistry, opts *Options) (*Kubernetes, error) {
	if opts == nil {
		opts = Defaults()
	}

	// this sets the internal logger that the kubernetes client uses
	klogv2.SetLogger(log.FromHCLogger(logger.Named("kubernetes-client")))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      opts.MetricsBindAddr,
		HealthProbeBindAddress:  opts.HealthProbeBindAddr,
		Port:                    opts.WebhookPort,
		LeaderElection:          true,
		LeaderElectionID:        controllerLeaderElectionID,
		LeaderElectionNamespace: "default",
		Logger:                  log.FromHCLogger(logger.Named("controller-runtime")),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to start k8s controller manager: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	if opts.CACertSecret != "" && opts.CACertFile != "" {
		client, err := clientruntime.New(ctrl.GetConfigOrDie(), clientruntime.Options{
			Scheme: scheme,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get k8s client: %w", err)
		}
		secret := &corev1.Secret{}
		err = client.Get(context.Background(), clientruntime.ObjectKey{
			Namespace: opts.CACertSecretNamespace,
			Name:      opts.CACertSecret,
		}, secret)
		if err != nil {
			return nil, fmt.Errorf("unable to pull Consul CA cert from secret: %w", err)
		}
		cert := secret.Data["tls.crt"]
		if err := os.WriteFile(opts.CACertFile, cert, 0444); err != nil {
			return nil, fmt.Errorf("unable to write CA cert: %w", err)
		}
	}

	return &Kubernetes{
		k8sManager:    mgr,
		registry:      registry,
		sDSServerHost: opts.SDSServerHost,
		sDSServerPort: opts.SDSServerPort,
		logger:        logger.Named("k8s"),
	}, nil
}

func (k *Kubernetes) SetConsul(consul *api.Client) {
	k.consul = consul
}

// Start will run the kubernetes controllers and return a startup error if occurred
func (k *Kubernetes) Start(ctx context.Context) error {
	k.logger.Trace("running controller")

	klogger := log.FromHCLogger(k.logger)

	reconcileManager := reconciler.NewReconcileManager(ctx, &reconciler.ManagerConfig{
		ControllerName: ControllerName,
		Registry:       k.registry,
		Client:         k.k8sManager.GetClient(),
		Consul:         k.consul,
		Status:         k.k8sManager.GetClient().Status(),
		Logger:         k.logger.Named("consul"),
	})

	err := (&controllers.GatewayClassConfigReconciler{
		Client: k.k8sManager.GetClient(),
		Log:    klogger.WithName("controllers").WithName("GatewayClassConfig"),
		Scheme: k.k8sManager.GetScheme(),
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway class config controller: %w", err)
	}

	err = (&controllers.GatewayClassReconciler{
		ControllerName: ControllerName,
		Client:         k.k8sManager.GetClient(),
		Log:            klogger.WithName("controllers").WithName("GatewayClass"),
		Scheme:         k.k8sManager.GetScheme(),
		Manager:        reconcileManager,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway class controller: %w", err)
	}

	err = (&controllers.GatewayReconciler{
		SDSServerHost:  k.sDSServerHost,
		SDSServerPort:  k.sDSServerPort,
		ControllerName: ControllerName,
		Tracker:        utils.NewStatusTracker(),
		Client:         k.k8sManager.GetClient(),
		Log:            klogger.WithName("controllers").WithName("Gateway"),
		Scheme:         k.k8sManager.GetScheme(),
		Manager:        reconcileManager,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway controller: %w", err)
	}

	err = (&controllers.HTTPRouteReconciler{
		Client:         k.k8sManager.GetClient(),
		Log:            klogger.WithName("controllers").WithName("HTTPRoute"),
		Scheme:         k.k8sManager.GetScheme(),
		Manager:        reconcileManager,
		ControllerName: ControllerName,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create http route controller: %w", err)
	}

	return k.k8sManager.Start(ctx)
}
