package state

import (
	"context"
	"sync"
)

const (
	defaultListenerName = "default"
)

// boundListener wraps a lstener and its set of routes
type listenerState struct {
	Listener

	gateway Gateway

	name     string
	hostname string
	port     int
	protocol string

	tlsResolved  bool
	certificates []string

	routes map[string]ResolvedRoute

	needsSync bool

	mutex sync.RWMutex
}

func newListenerState(gateway Gateway, listener Listener) *listenerState {
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

	return &listenerState{
		Listener:    listener,
		gateway:     gateway,
		name:        name,
		port:        listenerConfig.Port,
		protocol:    listenerConfig.Protocol,
		hostname:    hostname,
		tlsResolved: tlsResolved,
		routes:      make(map[string]ResolvedRoute),
		needsSync:   true,
	}
}

func (l *listenerState) RemoveRoute(id string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, found := l.routes[id]; !found {
		return
	}
	l.Logger().Trace("removing route from listener", "route", id)

	l.needsSync = true
	delete(l.routes, id)
}

func (l *listenerState) SetRoute(route Route) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.Logger().Trace("setting route on listener", "route", route.ID())
	if resolved := route.Resolve(l.Listener); resolved != nil {
		l.routes[route.ID()] = *resolved
		l.needsSync = true
	}
}

func (l *listenerState) ResolveAndCacheTLS(ctx context.Context) ([]string, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.tlsResolved {
		return l.certificates, nil
	}

	certificates, err := l.Listener.Certificates(ctx)
	if err != nil {
		return nil, err
	}

	l.certificates = certificates
	l.tlsResolved = true

	return certificates, nil
}

func (l *listenerState) ShouldSync() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.needsSync
}

func (l *listenerState) MarkSynced() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.needsSync = false
}

func (l *listenerState) Resolve() ResolvedListener {
	routes := []ResolvedRoute{}
	for _, route := range l.routes {
		routes = append(routes, route)
	}
	return ResolvedListener{
		Name:         l.name,
		Hostname:     l.hostname,
		Port:         l.port,
		Protocol:     l.protocol,
		Certificates: l.certificates,
		Routes:       routes,
	}
}
