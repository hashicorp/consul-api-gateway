package k8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/k8s/controllers"
	"github.com/hashicorp/polar/k8s/log"
	"github.com/hashicorp/polar/k8s/object"
	"github.com/hashicorp/polar/k8s/reconciler"
)

var (
	scheme = runtime.NewScheme()
)

const (
	polarLeaderElectionID = "polar.consul.hashicorp.com"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gw.AddToScheme(scheme))
}

type Kubernetes struct {
	k8sManager ctrl.Manager
	consul     *api.Client
	logger     hclog.Logger
	k8sStatus  *object.StatusWorker

	failed chan struct{}
}

type Options struct {
	MetricsBindAddr     string
	HealthProbeBindAddr string
	WebhookPort         int
}

func Defaults() *Options {
	return &Options{
		MetricsBindAddr:     ":8080",
		HealthProbeBindAddr: ":8081",
		WebhookPort:         8443,
	}
}

func New(client *api.Client, logger hclog.Logger, opts *Options) (*Kubernetes, error) {
	if opts == nil {
		opts = Defaults()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      opts.MetricsBindAddr,
		HealthProbeBindAddress:  opts.HealthProbeBindAddr,
		Port:                    opts.WebhookPort,
		LeaderElection:          true,
		LeaderElectionID:        polarLeaderElectionID,
		LeaderElectionNamespace: "default",
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

	return &Kubernetes{
		k8sManager: mgr,
		consul:     client,
		logger:     logger.Named("k8s"),
		failed:     make(chan struct{}),
	}, nil

}

// Start will run the kubernetes controllers and return a startup error if occurred
func (k *Kubernetes) Start(ctx context.Context) error {

	status := object.NewStatusWorker(ctx, k.k8sManager.GetClient().Status(), k.logger)
	k.k8sStatus = status

	klogger := log.FromHCLogger(k.logger)

	consulMgr := reconciler.NewReconcileManager(ctx, k.consul, k.k8sManager.GetClient().Status(), k.logger.Named("consul"))
	err := (&controllers.GatewayReconciler{
		Client:  k.k8sManager.GetClient(),
		Log:     klogger.WithName("controllers").WithName("Gateway"),
		Scheme:  k.k8sManager.GetScheme(),
		Manager: consulMgr,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway controller: %w", err)
	}

	err = (&controllers.HTTPRouteReconciler{
		Client:  k.k8sManager.GetClient(),
		Log:     klogger.WithName("controllers").WithName("HTTPRoute"),
		Scheme:  k.k8sManager.GetScheme(),
		Manager: consulMgr,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create http_route controller: %w", err)
	}

	go func() {
		err := k.k8sManager.Start(ctx)
		if err != nil {
			k.logger.Error("fatal controller error occurred", "error", err)
			close(k.failed)
		}
	}()

	return nil
}

// Failed returns a channel which will be closed if a critical failure occurs with the controller
func (k *Kubernetes) Failed() <-chan struct{} {
	return k.failed
}
