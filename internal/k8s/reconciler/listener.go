package reconciler

import (
	"sync/atomic"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
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

var _ store.Listener = &K8sListener{}

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

func (l *K8sListener) RouteRemoved() {
	atomic.AddInt32(&l.ListenerState.RouteCount, -1)
}

func (l *K8sListener) Status() gw.ListenerStatus {
	return l.GetStatus(l.listener, l.gateway.Generation)
}

func (l *K8sListener) IsValid() bool {
	return l.ListenerState.ValidWithProtocol(l.listener.Protocol)
}
