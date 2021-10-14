package reconciler

import (
	"context"
	"errors"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type K8sGatewayClasses struct {
	logger hclog.Logger
	client gatewayclient.Client

	gatewayClasses map[string]*K8sGatewayClass
	mutex          sync.RWMutex
}

func NewK8sGatewayClasses(logger hclog.Logger, client gatewayclient.Client) *K8sGatewayClasses {
	return &K8sGatewayClasses{
		logger:         logger,
		client:         client,
		gatewayClasses: make(map[string]*K8sGatewayClass),
	}
}

func (g *K8sGatewayClasses) GetConfig(name string) (apigwv1alpha1.GatewayClassConfig, bool) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	if class, found := g.gatewayClasses[name]; found {
		if class.IsValid() {
			return class.config, true
		}
		// pretend like we don't exist since a gateway
		// can't use an invalid gatewayclass and we're
		// supposed to use only a snapshot of the
		// gatewayclass at gateway creation time
	}
	return apigwv1alpha1.GatewayClassConfig{}, false
}

func (g *K8sGatewayClasses) Upsert(ctx context.Context, class *K8sGatewayClass) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if current, ok := g.gatewayClasses[class.class.Name]; ok {
		if utils.ResourceVersionGreater(current.class.ResourceVersion, class.class.ResourceVersion) {
			// we have an old gatewayclass update ignore
			return nil
		}
	}

	g.gatewayClasses[class.class.Name] = class

	return class.SyncStatus(ctx)
}

func (g *K8sGatewayClasses) Delete(name string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	delete(g.gatewayClasses, name)
}

type K8sGatewayClass struct {
	logger hclog.Logger
	client gatewayclient.Client

	status GatewayClassStatus
	class  *gw.GatewayClass
	config apigwv1alpha1.GatewayClassConfig
}

type K8sGatewayClassConfig struct {
	Logger hclog.Logger
	Client gatewayclient.Client
}

func NewK8sGatewayClass(class *gw.GatewayClass, config K8sGatewayClassConfig) *K8sGatewayClass {
	classLogger := config.Logger.Named("gatewayclass").With("name", class.Name)
	return &K8sGatewayClass{
		logger: classLogger,
		client: config.Client,
		class:  class,
	}
}

func (c *K8sGatewayClass) IsValid() bool {
	return !c.status.Accepted.HasError()
}

func (c *K8sGatewayClass) Validate(ctx context.Context) error {
	// only validate if we actually have a config reference
	if ref := c.class.Spec.ParametersRef; ref != nil {
		// check that we're using a typed config
		if ref.Group != apigwv1alpha1.Group || ref.Kind != apigwv1alpha1.GatewayClassConfigKind {
			c.status.Accepted.InvalidParameters = errors.New("unsupported gateway class configuration")
			return nil
		}

		// ignore namespace since we're cluster-scoped
		found, err := c.client.GetGatewayClassConfig(ctx, types.NamespacedName{Name: ref.Name})
		if err != nil {
			return err
		}
		if found == nil {
			c.status.Accepted.InvalidParameters = errors.New("gateway class not found")
			return nil
		}
		c.config = *found
		// clear out any accepted errors
		c.status.Accepted = GatewayClassAcceptedStatus{}
	}

	return nil
}

func (c *K8sGatewayClass) SyncStatus(ctx context.Context) error {
	current := c.class.Status.Conditions
	conditions := c.status.Conditions(c.class.Generation)
	if !conditionsEqual(conditions, current) {
		c.class.Status.Conditions = conditions
		if err := c.client.UpdateStatus(ctx, c.class); err != nil {
			// unset to sync on next retry
			c.class.Status.Conditions = current
			return err
		}
	}
	return nil
}
