package reconciler

import (
	"context"
	"sync/atomic"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
)

var (
	supportedProtocols = map[gw.ProtocolType][]gw.RouteGroupKind{
		gw.HTTPProtocolType: {{
			Group: (*gw.Group)(&gw.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		gw.HTTPSProtocolType: {{
			Group: (*gw.Group)(&gw.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		gw.TCPProtocolType: {{
			Group: (*gw.Group)(&gw.GroupVersion.Group),
			Kind:  "TCPRoute",
		}},
	}
)

const (
	defaultListenerName = "default"
)

type K8sListener struct {
	*ListenerState
	listener gw.Listener

	logger  hclog.Logger
	gateway *K8sGateway
	client  gatewayclient.Client
}

var _ store.RouteTrackingListener = &K8sListener{}

type K8sListenerConfig struct {
	Logger hclog.Logger
	Client gatewayclient.Client
}

func NewK8sListener(gateway *K8sGateway, listener gw.Listener, config K8sListenerConfig) *K8sListener {
	listenerLogger := config.Logger.Named("listener").With("listener", string(listener.Name))

	return &K8sListener{
		ListenerState: &ListenerState{},
		logger:        listenerLogger,
		client:        config.Client,
		gateway:       gateway,
		listener:      listener,
	}
}

func (l *K8sListener) ID() string {
	return string(l.listener.Name)
}

func (l *K8sListener) Config() store.ListenerConfig {
	name := defaultListenerName
	if l.listener.Name != "" {
		name = string(l.listener.Name)
	}
	hostname := ""
	if l.listener.Hostname != nil {
		hostname = string(*l.listener.Hostname)
	}
	protocol, tls := utils.ProtocolToConsul(l.listener.Protocol)

	// Update listener TLS config to specify whether TLS is required by the protocol
	l.ListenerState.TLS.Enabled = tls

	return store.ListenerConfig{
		Name:     name,
		Hostname: hostname,
		Port:     int(l.listener.Port),
		Protocol: protocol,
		TLS:      l.ListenerState.TLS,
	}
}

// CanBind returns whether a route can bind
// to a gateway, if the route can bind to a listener
// on the gateway the return value is nil, if not,
// an error specifying why the route cannot bind
// is returned.
func (l *K8sListener) CanBind(ctx context.Context, route store.Route) (bool, error) {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		l.logger.Error("route is not a known type")
		return false, nil
	}

	return (&Binder{
		Client:        l.client,
		Gateway:       l.gateway.Gateway,
		Listener:      l.listener,
		ListenerState: l.ListenerState,
	}).CanBind(ctx, k8sRoute)
}

func (l *K8sListener) OnRouteAdded(_ store.Route) {
	atomic.AddInt32(&l.ListenerState.RouteCount, 1)
}

func (l *K8sListener) OnRouteRemoved(_ string) {
	atomic.AddInt32(&l.ListenerState.RouteCount, -1)
}

func (l *K8sListener) Status() gw.ListenerStatus {
	return l.GetStatus(l.listener, l.gateway.Generation)
}

func (l *K8sListener) IsValid() bool {
	return l.ListenerState.ValidWithProtocol(l.listener.Protocol)
}

func supportedKindsFor(protocol gw.ProtocolType) []gw.RouteGroupKind {
	return supportedProtocols[protocol]
}

func kindsNotInSet(set, parent []gw.RouteGroupKind) []gw.RouteGroupKind {
	kinds := []gw.RouteGroupKind{}
	for _, kind := range set {
		if !isKindInSet(kind, parent) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func isKindInSet(value gw.RouteGroupKind, set []gw.RouteGroupKind) bool {
	for _, kind := range set {
		groupsMatch := false
		if value.Group == nil && kind.Group == nil {
			groupsMatch = true
		} else if value.Group != nil && kind.Group != nil && *value.Group == *kind.Group {
			groupsMatch = true
		}
		if groupsMatch && value.Kind == kind.Kind {
			return true
		}
	}
	return false
}
