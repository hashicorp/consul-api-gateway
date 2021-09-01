package k8s

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	clientruntime "sigs.k8s.io/controller-runtime/pkg/client"
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
	k8sManager   ctrl.Manager
	consul       *api.Client
	logger       hclog.Logger
	k8sStatus    *object.StatusWorker
	caCertSecret string

	failed chan struct{}
}

type Options struct {
	CACertSecret        string
	CACertFile          string
	MetricsBindAddr     string
	HealthProbeBindAddr string
	WebhookPort         int
}

func Defaults() *Options {
	return &Options{
		CACertSecret:        "",
		CACertFile:          "",
		MetricsBindAddr:     ":8080",
		HealthProbeBindAddr: ":8081",
		WebhookPort:         8443,
	}
}

func (o *Options) SetCACertSecret(secret string) *Options {
	o.CACertSecret = secret
	return o
}

func (o *Options) SetCACertFile(file string) *Options {
	o.CACertFile = file
	return o
}

func New(logger hclog.Logger, opts *Options) (*Kubernetes, error) {
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

	if opts.CACertSecret != "" && opts.CACertFile != "" {
		client, err := clientruntime.New(ctrl.GetConfigOrDie(), clientruntime.Options{
			Scheme: scheme,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get k8s client: %w", err)
		}
		secret := &corev1.Secret{}
		err = client.Get(context.Background(), clientruntime.ObjectKey{
			Namespace: "default",
			Name:      opts.CACertSecret,
		}, secret)
		if err != nil {
			return nil, fmt.Errorf("unable to pull Consul CA cert from secret: %w", err)
		}
		cert := secret.Data["tls.crt"]
		os.WriteFile(opts.CACertFile, cert, 0444)
	}

	return &Kubernetes{
		caCertSecret: opts.CACertSecret,
		k8sManager:   mgr,
		logger:       logger.Named("k8s"),
		failed:       make(chan struct{}),
	}, nil
}

func (k *Kubernetes) SetConsul(consul *api.Client) {
	k.consul = consul
}

// Start will run the kubernetes controllers and return a startup error if occurred
func (k *Kubernetes) Start(ctx context.Context) error {

	status := object.NewStatusWorker(ctx, k.k8sManager.GetClient().Status(), k.logger)
	k.k8sStatus = status

	klogger := log.FromHCLogger(k.logger)

	consulMgr := reconciler.NewReconcileManager(ctx, k.consul, k.k8sManager.GetClient().Status(), k.logger.Named("consul"))
	err := (&controllers.GatewayReconciler{
		Client:       k.k8sManager.GetClient(),
		Log:          klogger.WithName("controllers").WithName("Gateway"),
		Scheme:       k.k8sManager.GetScheme(),
		Manager:      consulMgr,
		CACertSecret: k.caCertSecret,
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
