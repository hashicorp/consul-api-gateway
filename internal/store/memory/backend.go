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

func (b *Backend) GetGateway(ctx context.Context, id core.GatewayID) ([]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if data, found := b.gateways[id]; found {
		return data, nil
	}
	return nil, store.ErrNotFound
}

func (b *Backend) GetGateways(ctx context.Context) ([][]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	gateways := [][]byte{}
	for _, data := range b.gateways {
		gateways = append(gateways, data)
	}
	return gateways, nil
}

func (b *Backend) DeleteGateway(ctx context.Context, id core.GatewayID) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.gateways, id)
	return nil
}

func (b *Backend) UpsertGateways(ctx context.Context, gateways ...store.GatewayRecord) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, gateway := range gateways {
		b.gateways[gateway.ID] = gateway.Data
	}
	return nil
}

func (b *Backend) GetRoute(ctx context.Context, id string) ([]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if data, found := b.routes[id]; found {
		return data, nil
	}
	return nil, store.ErrNotFound
}

func (b *Backend) GetRoutes(ctx context.Context) ([][]byte, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	routes := [][]byte{}
	for _, data := range b.routes {
		routes = append(routes, data)
	}
	return routes, nil
}

func (b *Backend) DeleteRoute(ctx context.Context, id string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.routes, id)
	return nil
}

func (b *Backend) UpsertRoutes(ctx context.Context, routes ...store.RouteRecord) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, route := range routes {
		b.routes[route.ID] = route.Data
	}
	return nil
}
