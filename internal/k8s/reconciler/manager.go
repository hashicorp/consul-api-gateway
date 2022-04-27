package reconciler

import (
	"context"
	"encoding/json"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/validators"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

//go:generate mockgen -source ./manager.go -destination ./mocks/manager.go -package mocks ReconcileManager

const (
	annotationConfig = "api-gateway.consul.hashicorp.com/config"
)

type ReconcileManager interface {
	UpsertGatewayClass(ctx context.Context, gc *gw.GatewayClass) error
	UpsertGateway(ctx context.Context, g *gw.Gateway) error
	UpsertHTTPRoute(ctx context.Context, r *gw.HTTPRoute) error
	UpsertTCPRoute(ctx context.Context, r *gw.TCPRoute) error
	DeleteGatewayClass(ctx context.Context, name string) error
	DeleteGateway(ctx context.Context, name types.NamespacedName) error
	DeleteHTTPRoute(ctx context.Context, name types.NamespacedName) error
	DeleteTCPRoute(ctx context.Context, name types.NamespacedName) error
}

// GatewayReconcileManager manages a GatewayReconciler for each Gateway and is the interface by which Consul operations
// should be invoked in a kubernetes controller.
type GatewayReconcileManager struct {
	logger hclog.Logger
	client gatewayclient.Client

	store                 store.Store
	gatewayClasses        *K8sGatewayClasses
	gatewayValidator      *validators.GatewayValidator
	routeValidator        *validators.RouteValidator
	deployer              *GatewayDeployer
	statusUpdater         *StatusUpdater
	consulNamespaceMapper common.ConsulNamespaceMapper

	namespaceMap map[types.NamespacedName]string
	// guards the above map
	mutex sync.RWMutex
}

var _ ReconcileManager = &GatewayReconcileManager{}

type ManagerConfig struct {
	ControllerName        string
	Client                gatewayclient.Client
	Consul                *api.Client
	ConsulCA              string
	SDSHost               string
	SDSPort               int
	Store                 store.Store
	Logger                hclog.Logger
	ConsulNamespaceMapper common.ConsulNamespaceMapper
}

func NewReconcileManager(config ManagerConfig) *GatewayReconcileManager {
	resolver := service.NewBackendResolver(config.Logger, config.ConsulNamespaceMapper, config.Client, config.Consul)
	deployer := NewDeployer(DeployerConfig{
		ConsulCA: config.ConsulCA,
		SDSHost:  config.SDSHost,
		SDSPort:  config.SDSPort,
		Logger:   config.Logger,
		Client:   config.Client,
	})
	statusUpdater := NewStatusUpdater(config.Logger, config.Client, deployer, config.ControllerName)

	return &GatewayReconcileManager{
		logger:                config.Logger,
		client:                config.Client,
		statusUpdater:         statusUpdater,
		deployer:              deployer,
		gatewayValidator:      validators.NewGatewayValidator(config.ConsulNamespaceMapper, config.Client),
		routeValidator:        validators.NewRouteValidator(resolver, config.Client),
		gatewayClasses:        NewK8sGatewayClasses(config.Logger.Named("gatewayclasses"), config.Client),
		namespaceMap:          make(map[types.NamespacedName]string),
		consulNamespaceMapper: config.ConsulNamespaceMapper,
		store:                 config.Store,
	}
}

func (m *GatewayReconcileManager) UpsertGatewayClass(ctx context.Context, gc *gw.GatewayClass) error {
	class := NewK8sGatewayClass(gc, K8sGatewayClassConfig{
		Logger: m.logger,
		Client: m.client,
	})

	if err := class.Validate(ctx); err != nil {
		return err
	}

	return m.gatewayClasses.Upsert(ctx, class)
}

func (m *GatewayReconcileManager) UpsertGateway(ctx context.Context, g *gw.Gateway) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var err error
	var config apigwv1alpha1.GatewayClassConfig
	var managed bool

	gatewayClassName := string(g.Spec.GatewayClassName)

	if g.Annotations == nil {
		g.Annotations = map[string]string{}
	}

	annotated := false

	// first check to see whether we have our initial configuration as a gateway annotation
	if annotatedConfig, ok := g.Annotations[annotationConfig]; ok {
		if err := json.Unmarshal([]byte(annotatedConfig), &config.Spec); err != nil {
			m.logger.Warn("error unmarshaling GatewayClassConfig annotation, skipping")
		} else {
			annotated = true
			managed = true
		}
	}

	if !managed {
		// next check our in-memory cache of class configs
		config, managed = m.gatewayClasses.GetConfig(gatewayClassName)
		if !managed {
			// finally, see if we can run through all of the relationships and retrieve the config
			config, managed, err = m.client.GetConfigForGatewayClassName(ctx, gatewayClassName)
			if err != nil {
				return err
			}
			if !managed {
				// we don't own the gateway
				return nil
			}
		}
	}

	// at this point we have a managed gateway and its configuration, make sure we set the
	// configuration annotation
	marshaled, err := json.Marshal(config.Spec)
	if err != nil {
		return err
	}
	g.Annotations[annotationConfig] = string(marshaled)
	if !annotated {
		// if we're annotating the gateway for the first time, update the gateway and return
		// the upsert will get handled next reconciliation loop which should get triggered
		// because of the call to Update
		return m.client.Update(ctx, g)
	}

	// Calling validate outside of the upsert process allows us to re-resolve any
	// external references and set the statuses accordingly.
	consulNamespace := m.consulNamespaceMapper(g.GetNamespace())
	service := m.deployer.Service(config, g)
	state, err := m.gatewayValidator.Validate(ctx, g, service)
	if err != nil {
		return err
	}

	m.namespaceMap[utils.NamespacedName(g)] = consulNamespace
	gateway := NewGateway(config, g, state)

	return m.store.UpsertGateway(ctx, gateway, func(current store.Gateway) bool {
		if current == nil {
			return true
		}
		return !utils.ResourceVersionGreater(current.(*K8sGateway).ResourceVersion, gateway.ResourceVersion)
	})
}

func (m *GatewayReconcileManager) UpsertHTTPRoute(ctx context.Context, r *gw.HTTPRoute) error {
	return m.upsertRoute(ctx, r, r.Spec.ParentRefs, HTTPRouteID(utils.NamespacedName(r)))
}

func (m *GatewayReconcileManager) UpsertTCPRoute(ctx context.Context, r *gw.TCPRoute) error {
	return m.upsertRoute(ctx, r, r.Spec.ParentRefs, TCPRouteID(utils.NamespacedName(r)))
}

func (m *GatewayReconcileManager) upsertRoute(ctx context.Context, r Route, parents []gw.ParentRef, id string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	managed, err := m.deleteUnmanagedRoute(ctx, r.GetNamespace(), parents, id)
	if err != nil {
		return err
	}
	if !managed {
		return nil
	}

	// Calling validate outside of the upsert process allows us to re-resolve any
	// external references and set the statuses accordingly.
	state, err := m.routeValidator.Validate(ctx, r)
	if err != nil {
		return err
	}

	route := NewRoute(r, state)

	return m.store.UpsertRoute(ctx, route, func(current store.Route) bool {
		if current == nil {
			return true
		}
		return !utils.ResourceVersionGreater(current.(*K8sRoute).GetResourceVersion(), route.GetResourceVersion())
	})
}

func (m *GatewayReconcileManager) DeleteGatewayClass(ctx context.Context, name string) error {
	m.gatewayClasses.Delete(name)
	return nil
}

func (m *GatewayReconcileManager) DeleteGateway(ctx context.Context, name types.NamespacedName) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if err := m.store.DeleteGateway(ctx, core.GatewayID{
		Service:         name.Name,
		ConsulNamespace: m.namespaceMap[name],
	}); err != nil {
		return err
	}

	delete(m.namespaceMap, name)

	return nil
}

func (m *GatewayReconcileManager) DeleteHTTPRoute(ctx context.Context, name types.NamespacedName) error {
	return m.store.DeleteRoute(ctx, HTTPRouteID(name))
}

func (m *GatewayReconcileManager) DeleteTCPRoute(ctx context.Context, name types.NamespacedName) error {
	return m.store.DeleteRoute(ctx, TCPRouteID(name))
}

func (m *GatewayReconcileManager) deleteUnmanagedRoute(ctx context.Context, namespace string, refs []gw.ParentRef, id string) (bool, error) {
	// check our cache first
	managed := m.managedByCachedGatewaysForRoute(namespace, refs)
	if !managed {
		var err error
		// we might not yet have the gateway in our cache, check remotely
		if managed, err = m.client.IsManagedRoute(ctx, namespace, refs); err != nil {
			return false, err
		}
	}

	if !managed {
		// we're not managing this route (potentially reference got removed on an update)
		// ensure it's cleaned up
		if err := m.store.DeleteRoute(ctx, id); err != nil {
			return false, err
		}
		return false, nil
	}

	return true, nil
}

func (m *GatewayReconcileManager) managedByCachedGatewaysForRoute(namespace string, parents []gw.ParentRef) bool {
	for _, parent := range parents {
		name, isGateway := utils.ReferencesGateway(namespace, parent)
		if isGateway {
			if _, found := m.namespaceMap[name]; found {
				return true
			}
		}
	}
	return false
}
