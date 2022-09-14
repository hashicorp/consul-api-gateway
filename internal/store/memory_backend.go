package store

import (
	"context"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

type memoryBackend struct {
	gateways map[core.GatewayID][]byte
	routes   map[string][]byte

	mutex sync.RWMutex
}

var _ Backend = &memoryBackend{}

func NewMemoryBackend() *memoryBackend {
	return &memoryBackend{
		gateways: make(map[core.GatewayID][]byte),
		routes:   make(map[string][]byte),
	}
}

func (b *memoryBackend) GetGateway(_ context.Context, id core.GatewayID) ([]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if data, found := b.gateways[id]; found {
		return data, nil
	}
	return nil, ErrNotFound
}

func (b *memoryBackend) ListGateways(_ context.Context) ([][]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	gateways := make([][]byte, 0, len(b.gateways))
	for _, data := range b.gateways {
		gateways = append(gateways, data)
	}
	return gateways, nil
}

func (b *memoryBackend) DeleteGateway(_ context.Context, id core.GatewayID) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.gateways, id)
	return nil
}

func (b *memoryBackend) UpsertGateways(_ context.Context, gateways ...GatewayRecord) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, gateway := range gateways {
		b.gateways[gateway.ID] = gateway.Data
	}
	return nil
}

func (b *memoryBackend) GetRoute(_ context.Context, id string) ([]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if data, found := b.routes[id]; found {
		return data, nil
	}
	return nil, ErrNotFound
}

func (b *memoryBackend) ListRoutes(_ context.Context) ([][]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	routes := make([][]byte, 0, len(b.routes))
	for _, data := range b.routes {
		routes = append(routes, data)
	}
	return routes, nil
}

func (b *memoryBackend) DeleteRoute(_ context.Context, id string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.routes, id)
	return nil
}

func (b *memoryBackend) UpsertRoutes(_ context.Context, routes ...RouteRecord) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, route := range routes {
		b.routes[route.ID] = route.Data
	}
	return nil
}
