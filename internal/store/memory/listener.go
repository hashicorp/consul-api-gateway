package memory

import (
	"reflect"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
)

const (
	defaultListenerName = "default"
)

// boundListener wraps a lstener and its set of routes
type listenerState struct {
	store.Listener

	gateway store.Gateway

	logger   hclog.Logger
	name     string
	hostname string
	port     int
	protocol string

	routes map[string]core.ResolvedRoute

	needsSync bool
}

func newListenerState(logger hclog.Logger, gateway store.Gateway, listener store.Listener) *listenerState {
	listenerConfig := listener.Config()

	name := defaultListenerName
	if listenerConfig.Name != "" {
		name = string(listenerConfig.Name)
	}
	hostname := ""
	if listenerConfig.Hostname != "" {
		hostname = listenerConfig.Hostname
	}

	return &listenerState{
		Listener:  listener,
		gateway:   gateway,
		logger:    logger.With("listener", name),
		name:      name,
		port:      listenerConfig.Port,
		protocol:  listenerConfig.Protocol,
		hostname:  hostname,
		routes:    make(map[string]core.ResolvedRoute),
		needsSync: true,
	}
}

func (l *listenerState) RemoveRoute(id string) {
	if _, found := l.routes[id]; !found {
		return
	}
	l.logger.Trace("removing route from listener", "route", id)
	if tracker, ok := l.Listener.(store.RouteTrackingListener); ok {
		tracker.OnRouteRemoved(id)
	}

	l.needsSync = true
	delete(l.routes, id)
}

func (l *listenerState) SetRoute(route store.Route) {
	l.logger.Trace("setting route on listener", "route", route.ID())
	if resolved := route.Resolve(l.Listener); resolved != nil {
		stored, found := l.routes[route.ID()]
		if found && reflect.DeepEqual(stored, *resolved) {
			// don't bother updating if the route is the same
			return
		}
		if tracker, ok := l.Listener.(store.RouteTrackingListener); ok {
			if !found {
				tracker.OnRouteAdded(route)
			}
		}

		l.routes[route.ID()] = *resolved

		l.needsSync = true
	}
}

func (l *listenerState) ShouldSync() bool {
	return l.needsSync
}

func (l *listenerState) MarkSynced() {
	l.needsSync = false
}

func (l *listenerState) Resolve() core.ResolvedListener {
	routes := []core.ResolvedRoute{}
	for _, route := range l.routes {
		routes = append(routes, route)
	}
	return core.ResolvedListener{
		Name:         l.name,
		Hostname:     l.hostname,
		Port:         l.port,
		Protocol:     l.protocol,
		Certificates: l.Listener.Certificates(),
		Routes:       routes,
	}
}
