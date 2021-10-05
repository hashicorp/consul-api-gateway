package state

import (
	"context"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul/api"
)

const (
	defaultListenerName = "default"
)

// boundListener wraps a lstener and its set of routes
type BoundListener struct {
	Listener

	gateway Gateway

	name     string
	hostname string
	port     int
	protocol string

	tlsResolved bool
	tls         *api.GatewayTLSConfig

	routes map[string]Route

	needsSync bool

	mutex sync.RWMutex
}

func NewBoundListener(gateway Gateway, listener Listener) *BoundListener {
	listenerConfig := listener.Config()

	name := defaultListenerName
	if listenerConfig.Name != "" {
		name = string(listenerConfig.Name)
	}
	hostname := ""
	if listenerConfig.Hostname != "" {
		hostname = listenerConfig.Hostname
	}
	tlsResolved := false
	if !listenerConfig.TLS {
		// we don't need to resolve any cert references, just
		// consider them resolved already
		tlsResolved = true
	}

	return &BoundListener{
		Listener:    listener,
		gateway:     gateway,
		name:        name,
		port:        listenerConfig.Port,
		protocol:    listenerConfig.Protocol,
		hostname:    hostname,
		tlsResolved: tlsResolved,
		routes:      make(map[string]Route),
		needsSync:   true,
	}
}

func (l *BoundListener) RemoveRoute(id string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, found := l.routes[id]; !found {
		return
	}

	l.needsSync = true
	delete(l.routes, id)
}

func (l *BoundListener) SetRoute(route Route) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.Logger().Trace("setting route", "route", route.ID())

	l.routes[route.ID()] = route
	l.needsSync = true
}

func (l *BoundListener) ResolveAndCacheTLS(ctx context.Context) (*api.GatewayTLSConfig, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.tlsResolved {
		return l.tls, nil
	}

	config, err := l.Listener.ResolveTLS(ctx)
	if err != nil {
		return nil, err
	}

	l.tls = config
	l.tlsResolved = true

	return config, nil
}

func (l *BoundListener) ShouldSync() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.needsSync
}

func (l *BoundListener) MarkSynced() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.needsSync = false
}

func (l *BoundListener) DiscoveryChain() (api.IngressListener, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	services := []api.IngressService{}
	routers := consul.NewConfigEntryIndex(api.ServiceRouter)
	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	l.Logger().Trace("rendering listener discovery chain")
	if len(l.routes) == 0 {
		l.Logger().Trace("listener has no routes")
	}
	for _, route := range l.routes {
		service, router, splits, serviceDefaults := route.DiscoveryChain(l.Listener)
		if service != nil {
			services = append(services, *service)
			routers.Add(router)
			splitters.Merge(splits)
			defaults.Merge(serviceDefaults)
		}
	}
	return api.IngressListener{
		Port:     l.port,
		Protocol: l.protocol,
		Services: services,
		TLS:      l.tls,
	}, routers, splitters, defaults
}
