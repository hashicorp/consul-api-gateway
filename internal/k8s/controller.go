package k8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	klogv2 "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/controllers"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

// The following RBAC rules are for leader election
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;update;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update

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
	store         store.Store
	logger        hclog.Logger
}

type Config struct {
	CACertSecretNamespace string
	CACertSecret          string
	CACertFile            string
	SDSServerHost         string
	SDSServerPort         int
	MetricsBindAddr       string
	HealthProbeBindAddr   string
	WebhookPort           int
	RestConfig            *rest.Config
	Namespace             string
}

func Defaults() *Config {
	return &Config{
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

func New(logger hclog.Logger, config *Config) (*Kubernetes, error) {
	if config == nil {
		config = Defaults()
	}

	// this sets the internal logger that the kubernetes client uses
	klogv2.SetLogger(fromHCLogger(logger.Named("kubernetes-client")))
	opts := ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      config.MetricsBindAddr,
		HealthProbeBindAddress:  config.HealthProbeBindAddr,
		Port:                    config.WebhookPort,
		LeaderElection:          true,
		LeaderElectionID:        controllerLeaderElectionID,
		LeaderElectionNamespace: "default",
		Logger:                  fromHCLogger(logger.Named("controller-runtime")),
	}
	if config.Namespace != "" {
		opts.LeaderElectionNamespace = config.Namespace
	}
	mgr, err := ctrl.NewManager(config.RestConfig, opts)

	if err != nil {
		return nil, fmt.Errorf("failed to start k8s controller manager: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	if config.CACertSecret != "" && config.CACertFile != "" {
		if err := utils.WriteSecretCertFile(config.RestConfig, config.CACertSecret, config.CACertFile, config.CACertSecretNamespace); err != nil {
			return nil, fmt.Errorf("unable to write CA cert file: %w", err)
		}
	}

	return &Kubernetes{
		k8sManager:    mgr,
		sDSServerHost: config.SDSServerHost,
		sDSServerPort: config.SDSServerPort,
		logger:        logger.Named("k8s"),
	}, nil
}

func (k *Kubernetes) SetConsul(consul *api.Client) {
	k.consul = consul
}

func (k *Kubernetes) SetStore(store store.Store) {
	k.store = store
}

// Start will run the kubernetes controllers and return a startup error if occurred
func (k *Kubernetes) Start(ctx context.Context) error {
	k.logger.Trace("running controller")

	scheme := k.k8sManager.GetScheme()
	apigwv1alpha1.RegisterTypes(scheme)

	gwClient := gatewayclient.New(k.k8sManager.GetClient(), scheme, ControllerName)

	reconcileManager := reconciler.NewReconcileManager(reconciler.ManagerConfig{
		ControllerName: ControllerName,
		Client:         gwClient,
		Consul:         k.consul,
		SDSConfig: apigwv1alpha1.SDSConfig{
			Host: k.sDSServerHost,
			Port: k.sDSServerPort,
		},
		Logger: k.logger.Named("Reconciler"),
		Store:  k.store,
	})

	err := (&controllers.GatewayClassConfigReconciler{
		Client: gwClient,
		Log:    k.logger.Named("GatewayClassConfig"),
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway class config controller: %w", err)
	}

	err = (&controllers.GatewayClassReconciler{
		Client:         gwClient,
		Log:            k.logger.Named("GatewayClass"),
		Manager:        reconcileManager,
		ControllerName: ControllerName,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway class controller: %w", err)
	}

	err = (&controllers.GatewayReconciler{
		Client:         gwClient,
		Log:            k.logger.Named("Gateway"),
		Manager:        reconcileManager,
		ControllerName: ControllerName,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create gateway controller: %w", err)
	}

	err = (&controllers.HTTPRouteReconciler{
		Client:         gwClient,
		Log:            k.logger.Named("HTTPRoute"),
		Manager:        reconcileManager,
		ControllerName: ControllerName,
	}).SetupWithManager(k.k8sManager)
	if err != nil {
		return fmt.Errorf("failed to create http route controller: %w", err)
	}

	return k.k8sManager.Start(ctx)
}
