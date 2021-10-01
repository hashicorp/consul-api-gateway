package reconciler

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// boundGatewayListener wraps a lstener and its set of routes
type BoundListener struct {
	logger hclog.Logger

	gateway  *gw.Gateway
	listener gw.Listener

	name     string
	hostname string
	port     int
	protocol string
	tls      *api.GatewayTLSConfig

	routes map[types.NamespacedName]*K8sRoute

	needsSync bool

	mutex sync.RWMutex
}

func NewBoundListener(ctx context.Context, client gatewayclient.Client, logger hclog.Logger, gateway *gw.Gateway, listener gw.Listener) (*BoundListener, error) {
	name := defaultListenerName
	if listener.Name != "" {
		name = string(listener.Name)
	}

	protocol, tls := utils.ProtocolToConsul(listener.Protocol)

	l := &BoundListener{
		logger:    logger.Named("listener").With("listener", name),
		gateway:   gateway,
		listener:  listener,
		name:      name,
		port:      int(listener.Port),
		protocol:  protocol,
		routes:    make(map[types.NamespacedName]*K8sRoute),
		needsSync: true,
	}

	if listener.Hostname != nil {
		l.hostname = string(*listener.Hostname)
	}

	if tls {
		if listener.TLS == nil {
			return nil, ErrInvalidTLSConfiguration
		}

		if listener.TLS.Mode != nil && *listener.TLS.Mode == gw.TLSModePassthrough {
			return nil, ErrTLSPassthroughUnsupported
		}

		if listener.TLS.CertificateRef == nil {
			return nil, ErrInvalidTLSCertReference
		}

		resource, err := resolveCertificateReference(ctx, client, gateway, *listener.TLS.CertificateRef)
		if err != nil {
			return nil, err
		}
		l.tls = &api.GatewayTLSConfig{
			SDS: &api.GatewayTLSSDSConfig{
				ClusterName:  "sds-cluster",
				CertResource: resource,
			},
		}
	}

	return l, nil
}

func resolveCertificateReference(ctx context.Context, client gatewayclient.Client, gateway *gw.Gateway, certRef gw.SecretObjectReference) (string, error) {
	group := core.GroupName
	kind := "Secret"
	namespace := gateway.Namespace

	if certRef.Group != nil {
		group = string(*certRef.Group)
	}
	if certRef.Kind != nil {
		kind = string(*certRef.Kind)
	}
	if certRef.Namespace != nil {
		namespace = string(*certRef.Namespace)
	}

	switch {
	case kind == "Secret" && group == core.GroupName:
		cert, err := client.GetSecret(ctx, types.NamespacedName{Name: certRef.Name, Namespace: namespace})
		if err != nil {
			return "", fmt.Errorf("error fetching secret: %w", err)
		}
		if cert == nil {
			return "", ErrInvalidTLSCertReference
		}
		return utils.NewK8sSecret(namespace, certRef.Name).String(), nil
	// add more supported types here
	default:
		return "", ErrInvalidTLSCertReference
	}
}

func (l *BoundListener) RemoveRoute(namespacedName types.NamespacedName) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, found := l.routes[namespacedName]; !found {
		return
	}

	l.needsSync = true
	delete(l.routes, namespacedName)
}

func (l *BoundListener) SetRoute(route *K8sRoute) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.routes[utils.NamespacedName(route)] = route
	l.needsSync = true
}

func (l *BoundListener) ShouldSync() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.needsSync
}

func (l *BoundListener) SetSynced() {
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
	if len(l.routes) == 0 {
		l.logger.Debug("listener has no routes")
	}
	for _, route := range l.routes {
		meta := map[string]string{
			"managed_by":                                 "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":        l.gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace":   l.gateway.Namespace,
			"consul-api-gateway/k8s/HTTPRoute.Name":      route.GetName(),
			"consul-api-gateway/k8s/HTTPRoute.Namespace": route.GetNamespace(),
		}
		prefix := fmt.Sprintf("consul-api-gateway_%s_", l.gateway.Name)

		service, router, splits, serviceDefaults := route.DiscoveryChain(prefix, l.hostname, meta)
		if service != nil {
			services = append(services, *service)
			routers.Add(router)
			splitters.Merge(splits)
			defaults.Merge(serviceDefaults)
		} else {
			l.logger.Debug("route has no resolved service", "route", route.GetName())
		}
	}
	return api.IngressListener{
		Port:     l.port,
		Protocol: l.protocol,
		Services: services,
		TLS:      l.tls,
	}, routers, splitters, defaults
}
