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
	client gatewayclient.Client

	gateway  *gw.Gateway
	listener gw.Listener

	name     string
	hostname string
	port     int
	protocol string

	tlsResolved bool
	tls         *api.GatewayTLSConfig

	routes map[types.NamespacedName]*K8sRoute

	needsSync bool

	mutex sync.RWMutex
}

type BoundListenerConfig struct {
	Logger hclog.Logger
	Client gatewayclient.Client
}

func NewBoundListener(gateway *gw.Gateway, listener gw.Listener, config BoundListenerConfig) *BoundListener {
	name := defaultListenerName
	if listener.Name != "" {
		name = string(listener.Name)
	}
	hostname := ""
	if listener.Hostname != nil {
		hostname = string(*listener.Hostname)
	}
	protocol, tls := utils.ProtocolToConsul(listener.Protocol)
	tlsResolved := false
	if !tls {
		// we don't need to resolve any cert references, just
		// consider them resolved already
		tlsResolved = true
	}
	listenerLogger := config.Logger.Named("listener").With("listener", name)

	return &BoundListener{
		logger:      listenerLogger,
		client:      config.Client,
		gateway:     gateway,
		listener:    listener,
		name:        name,
		port:        int(listener.Port),
		protocol:    protocol,
		hostname:    hostname,
		tlsResolved: tlsResolved,
		routes:      make(map[types.NamespacedName]*K8sRoute),
		needsSync:   true,
	}
}

func (l *BoundListener) ResolveCertificates(ctx context.Context) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if !l.tlsResolved {
		if l.listener.TLS == nil {
			return ErrInvalidTLSConfiguration
		}

		if l.listener.TLS.Mode != nil && *l.listener.TLS.Mode == gw.TLSModePassthrough {
			return ErrTLSPassthroughUnsupported
		}

		if l.listener.TLS.CertificateRef == nil {
			return ErrInvalidTLSCertReference
		}

		ref := *l.listener.TLS.CertificateRef
		resource, err := l.resolveCertificateReference(ctx, ref)
		if err != nil {
			return err
		}
		l.tls = &api.GatewayTLSConfig{
			SDS: &api.GatewayTLSSDSConfig{
				ClusterName:  "sds-cluster",
				CertResource: resource,
			},
		}
		l.tlsResolved = true
	}
	return nil
}

func (l *BoundListener) resolveCertificateReference(ctx context.Context, ref gw.SecretObjectReference) (string, error) {
	group := core.GroupName
	kind := "Secret"
	namespace := l.gateway.Namespace

	if ref.Group != nil {
		group = string(*ref.Group)
	}
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	switch {
	case kind == "Secret" && group == core.GroupName:
		l.logger.Trace("fetching certificate secret", "secret.name", ref.Name, "secret.namespace", namespace)
		cert, err := l.client.GetSecret(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace})
		if err != nil {
			return "", fmt.Errorf("error fetching secret: %w", err)
		}
		if cert == nil {
			return "", fmt.Errorf("certificate not found: %w", ErrInvalidTLSCertReference)
		}
		return utils.NewK8sSecret(namespace, ref.Name).String(), nil
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

func (l *BoundListener) TryBind(ref gw.ParentRef, route *K8sRoute) (bool, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(l.listener.Name, ref.SectionName)
	if allowed {
		if !routeKindIsAllowedForListener(l.listener.AllowedRoutes, route) {
			if must {
				return false, fmt.Errorf("route kind not allowed for listener: %w", ErrCannotBindListener)
			}
			return false, nil
		}
		allowed, err := routeAllowedForListenerNamespaces(l.gateway.Namespace, l.listener.AllowedRoutes, route)
		if err != nil {
			return false, fmt.Errorf("error checking listener namespaces: %w", err)
		}
		if !allowed {
			if must {
				return false, fmt.Errorf("route not allowed because of listener namespace policy: %w", ErrCannotBindListener)
			}
			return false, nil
		}

		if !route.MatchesHostname(l.listener.Hostname) {
			if must {
				return false, fmt.Errorf("route does not match listener hostname: %w", ErrCannotBindListener)
			}
			return false, nil
		}

		l.routes[utils.NamespacedName(route)] = route
		l.needsSync = true
		return true, nil
	}

	return false, nil
}

func (l *BoundListener) DiscoveryChain() (api.IngressListener, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	services := []api.IngressService{}
	routers := consul.NewConfigEntryIndex(api.ServiceRouter)
	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	l.logger.Trace("rendering listener discovery chain")
	if len(l.routes) == 0 {
		l.logger.Trace("listener has no routes")
	}
	for _, route := range l.routes {
		meta := routeConsulMeta(l.gateway, route)
		prefix := fmt.Sprintf("consul-api-gateway_%s_", l.gateway.Name)
		service, router, splits, serviceDefaults := route.DiscoveryChain(prefix, l.hostname, meta)
		if service != nil {
			services = append(services, *service)
			routers.Add(router)
			splitters.Merge(splits)
			defaults.Merge(serviceDefaults)
		} else {
			l.logger.Trace("route has no resolved service", "route", route.GetName())
		}
	}
	return api.IngressListener{
		Port:     l.port,
		Protocol: l.protocol,
		Services: services,
		TLS:      l.tls,
	}, routers, splitters, defaults
}

func routeConsulMeta(gateway *gw.Gateway, route *K8sRoute) map[string]string {
	return map[string]string{
		"managed_by":                                 "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":        gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace":   gateway.Namespace,
		"consul-api-gateway/k8s/HTTPRoute.Name":      route.GetName(),
		"consul-api-gateway/k8s/HTTPRoute.Namespace": route.GetNamespace(),
	}
}
