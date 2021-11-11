package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
	}
)

const (
	defaultListenerName = "default"
)

type K8sListener struct {
	consulNamespace string
	logger          hclog.Logger
	gateway         *gw.Gateway
	listener        gw.Listener
	client          gatewayclient.Client

	status         ListenerStatus
	routeCount     int32
	certificates   []string
	supportedKinds []gw.RouteGroupKind
}

var _ store.RouteTrackingListener = &K8sListener{}

type K8sListenerConfig struct {
	ConsulNamespace string
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func NewK8sListener(gateway *gw.Gateway, listener gw.Listener, config K8sListenerConfig) *K8sListener {
	listenerLogger := config.Logger.Named("listener").With("listener", string(listener.Name))

	return &K8sListener{
		consulNamespace: config.ConsulNamespace,
		logger:          listenerLogger,
		client:          config.Client,
		gateway:         gateway,
		listener:        listener,
	}
}

func (l *K8sListener) ID() string {
	return string(l.listener.Name)
}

func (l *K8sListener) Certificates() []string {
	return l.certificates
}

func (l *K8sListener) Validate(ctx context.Context) error {
	l.validateUnsupported()
	l.validateProtocols()

	if err := l.validateTLS(ctx); err != nil {
		return err
	}

	if l.status.Ready.Invalid == nil && !l.status.Valid() {
		// set the listener as invalid if any other statuses are not valid
		l.status.Ready.Invalid = errors.New("listener is in an invalid state")
	}

	return nil
}

func (l *K8sListener) validateTLS(ctx context.Context) error {
	if l.listener.TLS == nil {
		if l.Config().TLS {
			// we are using a protocol that requires TLS but has no TLS
			// configured
			l.status.Ready.Invalid = errors.New("tls configuration required for the given protocol")
		}
		return nil
	}

	if l.listener.TLS.Mode != nil && *l.listener.TLS.Mode == gw.TLSModePassthrough {
		l.status.Ready.Invalid = errors.New("tls passthrough not supported")
		return nil
	}

	if len(l.listener.TLS.CertificateRefs) == 0 {
		l.status.ResolvedRefs.InvalidCertificateRef = errors.New("certificate reference must be set")
		return nil
	}

	// we only support a single certificate for now
	ref := *l.listener.TLS.CertificateRefs[0]
	resource, err := l.resolveCertificateReference(ctx, ref)
	if err != nil {
		var certificateErr CertificateResolutionError
		if !errors.As(err, &certificateErr) {
			return err
		}
		l.status.ResolvedRefs.InvalidCertificateRef = certificateErr
	} else {
		l.certificates = []string{resource}
	}

	return nil
}

func (l *K8sListener) validateUnsupported() {
	// seems weird that we're looking at gateway fields for listener status
	// but that's the weirdness of the spec
	if len(l.gateway.Spec.Addresses) > 0 {
		// we dnn't support address binding
		l.status.Detached.UnsupportedAddress = errors.New("specified addresses are not supported")
	}
}

func (l *K8sListener) validateProtocols() {
	supportedKinds, found := supportedProtocols[l.listener.Protocol]
	if !found {
		l.status.Detached.UnsupportedProtocol = fmt.Errorf("unsupported protocol: %s", l.listener.Protocol)
	}
	l.supportedKinds = supportedKinds
	if l.listener.AllowedRoutes != nil {
		remainderKinds := kindsNotInSet(l.listener.AllowedRoutes.Kinds, supportedKinds)
		if len(remainderKinds) != 0 {
			l.status.ResolvedRefs.InvalidRouteKinds = fmt.Errorf("listener has unsupported kinds: %v", remainderKinds)
		}
	}
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

func (l *K8sListener) resolveCertificateReference(ctx context.Context, ref gw.SecretObjectReference) (string, error) {
	group := corev1.GroupName
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
	case kind == "Secret" && group == corev1.GroupName:
		cert, err := l.client.GetSecret(ctx, types.NamespacedName{Name: string(ref.Name), Namespace: namespace})
		if err != nil {
			return "", fmt.Errorf("error fetching secret: %w", err)
		}
		if cert == nil {
			return "", NewCertificateResolutionErrorNotFound("certificate not found")
		}
		return utils.NewK8sSecret(namespace, string(ref.Name)).String(), nil
	// add more supported types here
	default:
		return "", NewCertificateResolutionErrorUnsupported(fmt.Sprintf("unsupported certificate type - group: %s, kind: %s", group, kind))
	}
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
	return store.ListenerConfig{
		Name:     name,
		Hostname: hostname,
		Port:     int(l.listener.Port),
		Protocol: protocol,
		TLS:      tls,
	}
}

// CanBind returns whether a route can bind
// to a gateway, if the route can bind to a listener
// on the gateway the return value is nil, if not,
// an error specifying why the route cannot bind
// is returned.
func (l *K8sListener) CanBind(route store.Route) (bool, error) {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		l.logger.Error("route is not a known type")
		return false, nil
	}

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		l.logger.Trace("checking route parent ref", "name", ref.Name)
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			expected := utils.NamespacedName(l.gateway)
			l.logger.Trace("checking gateway match", "expected", expected.String(), "found", namespacedName.String())
			if expected == namespacedName {
				canBind, err := l.canBind(ref, k8sRoute)
				if err != nil {
					return false, err
				}
				if canBind {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (l *K8sListener) canBind(ref gw.ParentRef, route *K8sRoute) (bool, error) {
	if l.status.Ready.HasError() {
		l.logger.Trace("listener not ready, unable to bind", "route", route.ID())
		return false, nil
	}

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(l.listener.Name, ref.SectionName)
	if allowed {
		if !routeKindIsAllowedForListener(l.supportedKinds, route) {
			l.logger.Trace("route kind not allowed for listener", "route", route.ID())
			if must {
				return false, NewBindErrorRouteKind("route kind not allowed for listener")
			}
			return false, nil
		}
		allowed, err := routeAllowedForListenerNamespaces(l.gateway.Namespace, l.listener.AllowedRoutes, route)
		if err != nil {
			return false, fmt.Errorf("error checking listener namespaces: %w", err)
		}
		if !allowed {
			l.logger.Trace("route not allowed because of listener namespace policy", "route", route.ID())
			if must {
				return false, NewBindErrorListenerNamespacePolicy("route not allowed because of listener namespace policy")
			}
			return false, nil
		}

		if !route.MatchesHostname(l.listener.Hostname) {
			l.logger.Trace("route does not match listener hostname", "route", route.ID())
			if must {
				return false, NewBindErrorHostnameMismatch("route does not match listener hostname")
			}
			return false, nil
		}

		return true, nil
	}

	l.logger.Trace("route does not match listener name", "route", route.ID())
	return false, nil
}

func (l *K8sListener) OnRouteAdded(_ store.Route) {
	atomic.AddInt32(&l.routeCount, 1)
}

func (l *K8sListener) OnRouteRemoved(_ string) {
	atomic.AddInt32(&l.routeCount, -1)
}

func (l *K8sListener) Status() gw.ListenerStatus {
	return gw.ListenerStatus{
		Name:           l.listener.Name,
		SupportedKinds: l.supportedKinds,
		AttachedRoutes: atomic.LoadInt32(&l.routeCount),
		Conditions:     l.status.Conditions(l.gateway.Generation),
	}
}
