package reconciler

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/k8s/object"
	"github.com/hashicorp/polar/k8s/routes"
	"github.com/hashicorp/polar/k8s/utils"
)

// GatewayReconcileManager manages a GatewayReconciler for each Gateway and is the interface by which Consul operations
// should be invoked in a kubernetes controller.
type GatewayReconcileManager struct {
	ctx    context.Context
	consul *api.Client
	routes *routes.KubernetesRoutes
	logger hclog.Logger
	status *object.StatusWorker

	reconcilersMu sync.Mutex
	reconcilers   map[types.NamespacedName]*GatewayReconciler
}

func NewReconcileManager(ctx context.Context, consul *api.Client, status client.StatusWriter, logger hclog.Logger) *GatewayReconcileManager {
	return &GatewayReconcileManager{
		ctx:         ctx,
		consul:      consul,
		reconcilers: map[types.NamespacedName]*GatewayReconciler{},
		routes:      routes.NewKubernetesRoutes(status),
		logger:      logger,
		status:      object.NewStatusWorker(ctx, status, logger.Named("status")),
	}
}

func (m *GatewayReconcileManager) UpsertGateway(g *gw.Gateway) {
	namespacedName := utils.KubeObjectNamespacedName(g)
	m.reconcilersMu.Lock()
	defer m.reconcilersMu.Unlock()
	r, ok := m.reconcilers[namespacedName]
	if !ok {
		r = newReconcilerForGateway(m.ctx, m.consul, m.logger, g, m.routes, m.status)
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

func (m *GatewayReconcileManager) DeleteGateway(name types.NamespacedName) {
	m.reconcilersMu.Lock()
	defer m.reconcilersMu.Unlock()
	r, ok := m.reconcilers[name]
	if !ok {
		return
	}

	r.stop()
	delete(m.reconcilers, name)
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
