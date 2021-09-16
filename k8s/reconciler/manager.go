package reconciler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/polar/internal/common"
	"github.com/hashicorp/polar/internal/metrics"
	"github.com/hashicorp/polar/k8s/object"
	"github.com/hashicorp/polar/k8s/routes"
	"github.com/hashicorp/polar/k8s/utils"
)

// GatewayReconcileManager manages a GatewayReconciler for each Gateway and is the interface by which Consul operations
// should be invoked in a kubernetes controller.
type GatewayReconcileManager struct {
	controllerName string
	ctx            context.Context
	registry       *common.GatewayRegistry
	consul         *api.Client
	routes         *routes.KubernetesRoutes
	logger         hclog.Logger
	status         *object.StatusWorker
	activeGateways int64

	reconcilersMu  sync.Mutex
	reconcilers    map[types.NamespacedName]*GatewayReconciler
	gatewayClasses map[string]*object.Object
}

type ManagerConfig struct {
	ControllerName string
	Registry       *common.GatewayRegistry
	Consul         *api.Client
	Status         client.StatusWriter
	Logger         hclog.Logger
}

func NewReconcileManager(ctx context.Context, config *ManagerConfig) *GatewayReconcileManager {
	return &GatewayReconcileManager{
		controllerName: config.ControllerName,
		ctx:            ctx,
		consul:         config.Consul,
		registry:       config.Registry,
		reconcilers:    map[types.NamespacedName]*GatewayReconciler{},
		gatewayClasses: map[string]*object.Object{},
		routes:         routes.NewKubernetesRoutes(),
		logger:         config.Logger,
		status:         object.NewStatusWorker(ctx, config.Status, config.Logger.Named("Status")),
	}
}

func (m *GatewayReconcileManager) UpsertGatewayClass(gc *gw.GatewayClass) {
	if gc.Spec.Controller == m.controllerName {
		var currentGen int64
		m.reconcilersMu.Lock()
		if current, ok := m.gatewayClasses[gc.Name]; ok {
			currentGen = current.GetGeneration()
		}
		if gc.Generation > currentGen {
			obj := object.New(gc)
			m.gatewayClasses[gc.Name] = obj
			obj.Status.Mutate(func(status interface{}) interface{} {
				gwcStatus := status.(*gw.GatewayClassStatus)
				gwcStatus.Conditions = []metav1.Condition{
					{
						Type:               string(gw.GatewayClassConditionStatusAdmitted),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: gc.Generation,
						LastTransitionTime: metav1.Now(),
						Reason:             string(gw.GatewayClassReasonAdmitted),
						Message:            fmt.Sprintf("admitted by controller %q", gc.Spec.Controller),
					},
				}

				return gwcStatus
			})
			if obj.Status.IsDirty() {
				m.status.Push(obj)
			}
		}
		m.reconcilersMu.Unlock()
	}

}

func (m *GatewayReconcileManager) UpsertGateway(g *gw.Gateway) {
	namespacedName := utils.KubeObjectNamespacedName(g)
	m.reconcilersMu.Lock()
	defer m.reconcilersMu.Unlock()

	// check that a matching gateway class exists
	if _, ok := m.gatewayClasses[g.Spec.GatewayClassName]; !ok {
		return
	}

	r, ok := m.reconcilers[namespacedName]
	if !ok {
		m.registry.AddGateway(common.GatewayInfo{
			Service:   g.GetName(),
			Namespace: g.GetNamespace(),
		}, referencedSecretsForGateway(g)...)
		activeGateways := atomic.AddInt64(&m.activeGateways, 1)
		metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))
		r = newReconcilerForGateway(m.ctx, &gatewayReconcilerArgs{
			controllerName: m.controllerName,
			consul:         m.consul,
			logger:         m.logger,
			gateway:        g,
			routes:         m.routes,
			status:         m.status,
		})
		go r.loop()
		m.reconcilers[namespacedName] = r
		m.logger.Debug("gateway inserted", "gateway", g.Name)
		r.signalReconcile()
		return
	}

	if r.kubeGateway.GetGeneration() != g.GetGeneration() {
		r.kubeGateway = g
		m.logger.Debug("gateway updated", "gateway", g.Name)
		r.signalReconcile()
	}
}

func (m *GatewayReconcileManager) UpsertHTTPRoute(r *gw.HTTPRoute) {
	if m.routes.Set(r) {
		m.logger.Debug("route upserted", "route", r.Name)
		m.signalAll()
	}
}

func (m *GatewayReconcileManager) DeleteGatewayClass(name string) {
	m.reconcilersMu.Lock()
	delete(m.gatewayClasses, name)
	m.reconcilersMu.Unlock()
}

func (m *GatewayReconcileManager) DeleteGateway(name types.NamespacedName) {
	m.reconcilersMu.Lock()
	defer m.reconcilersMu.Unlock()
	r, ok := m.reconcilers[name]
	if !ok {
		return
	}

	r.stop()
	delete(m.reconcilers, name)

	m.registry.RemoveGateway(common.GatewayInfo{
		Service:   name.Name,
		Namespace: name.Namespace,
	})
	activeGateways := atomic.AddInt64(&m.activeGateways, -1)
	metrics.Registry.SetGauge(metrics.K8sGateways, float32(activeGateways))
}

func (m *GatewayReconcileManager) DeleteRoute(name types.NamespacedName) {
	m.routes.Delete(name)
	m.signalAll()
}

func (m *GatewayReconcileManager) signalAll() {
	for _, r := range m.reconcilers {
		r.signalReconcile()
	}
}

func referencedSecretsForGateway(g *gw.Gateway) []string {
	secrets := []string{}
	for _, listener := range g.Spec.Listeners {
		if listener.TLS != nil {
			ref := listener.TLS.CertificateRef
			if ref != nil {
				n := ref.Namespace
				namespace := "default"
				if n != nil {
					namespace = *n
				}
				secrets = append(secrets, fmt.Sprintf("k8s://%s/%s", namespace, ref.Name))
			}
		}
	}
	return secrets
}
