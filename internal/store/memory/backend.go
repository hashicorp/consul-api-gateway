package memory

import (
	"context"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

type Backend struct {
	gateways map[core.GatewayID][]byte
	routes   map[string][]byte

	mutex sync.RWMutex
}

var _ store.Backend = &Backend{}

func NewBackend() *Backend {
	return &Backend{
		gateways: make(map[core.GatewayID][]byte),
		routes:   make(map[string][]byte),
	}
}

func (b *Backend) GetGateway(_ context.Context, id core.GatewayID) ([]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if data, found := b.gateways[id]; found {
		return data, nil
	}
	return nil, store.ErrNotFound
}

func (b *Backend) ListGateways(_ context.Context) ([][]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	gateways := make([][]byte, 0, len(b.gateways))
	for _, data := range b.gateways {
		gateways = append(gateways, data)
	}
	return gateways, nil
}

func (b *Backend) DeleteGateway(_ context.Context, id core.GatewayID) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.gateways, id)
	return nil
}

func (b *Backend) UpsertGateways(_ context.Context, gateways ...store.GatewayRecord) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, gateway := range gateways {
		b.gateways[gateway.ID] = gateway.Data
	}
	return nil
}

func (b *Backend) GetRoute(_ context.Context, id string) ([]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if data, found := b.routes[id]; found {
		return data, nil
	}
	return nil, store.ErrNotFound
}

func (b *Backend) ListRoutes(_ context.Context) ([][]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	routes := make([][]byte, 0, len(b.routes))
	for _, data := range b.routes {
		routes = append(routes, data)
	}
	return routes, nil
}

func (b *Backend) DeleteRoute(_ context.Context, id string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.routes, id)
	return nil
}

func (b *Backend) UpsertRoutes(_ context.Context, routes ...store.RouteRecord) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, route := range routes {
		b.routes[route.ID] = route.Data
	}
	return nil
}
