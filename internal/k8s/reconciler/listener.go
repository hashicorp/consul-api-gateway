package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	rcommon "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	rerrors "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

const (
	defaultListenerName = "default"
)

type K8sListener struct {
	consulNamespace string
	logger          hclog.Logger
	gateway         *gwv1beta1.Gateway
	listener        gwv1beta1.Listener
	client          gatewayclient.Client

	status         *rstatus.ListenerStatus
	tls            core.TLSParams
	routeCount     int32
	supportedKinds []gwv1beta1.RouteGroupKind
}

var _ store.RouteTrackingListener = &K8sListener{}

type K8sListenerConfig struct {
	ConsulNamespace string
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func NewK8sListener(gateway *gwv1beta1.Gateway, listener gwv1beta1.Listener, config K8sListenerConfig) *K8sListener {
	listenerLogger := config.Logger.Named("listener").With("listener", string(listener.Name))

	return &K8sListener{
		consulNamespace: config.ConsulNamespace,
		logger:          listenerLogger,
		client:          config.Client,
		gateway:         gateway,
		listener:        listener,
		status:          &rstatus.ListenerStatus{},
	}
}

func (l *K8sListener) ID() string {
	return string(l.listener.Name)
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
		// TODO: should this struct field be "Required" instead of "Enabled"?
		if l.Config().TLS.Enabled {
			// we are using a protocol that requires TLS but has no TLS
			// configured
			l.status.Ready.Invalid = errors.New("tls configuration required for the given protocol")
		}
		return nil
	}

	if l.listener.TLS.Mode != nil && *l.listener.TLS.Mode == gwv1beta1.TLSModePassthrough {
		l.status.Ready.Invalid = errors.New("tls passthrough not supported")
		return nil
	}

	if len(l.listener.TLS.CertificateRefs) == 0 {
		l.status.ResolvedRefs.InvalidCertificateRef = errors.New("certificate reference must be set")
		return nil
	}

	// we only support a single certificate for now
	ref := l.listener.TLS.CertificateRefs[0]

	// require ReferenceGrant for cross-namespace certificateRef
	allowed, err := gatewayAllowedForSecretRef(ctx, l.gateway, ref, l.client)
	if err != nil {
		return err
	} else if !allowed {
		nsName := getNamespacedName(ref.Name, ref.Namespace, l.gateway.Namespace)
		l.logger.Warn("Cross-namespace listener certificate not allowed without matching ReferenceGrant", "refName", nsName.Name, "refNamespace", nsName.Namespace)
		l.status.ResolvedRefs.InvalidCertificateRef = rerrors.NewCertificateResolutionErrorNotPermitted(
			fmt.Sprintf("Cross-namespace listener certificate not allowed without matching ReferenceGrant for Secret %q", nsName))
		return nil
	}

	resource, err := l.resolveCertificateReference(ctx, ref)
	if err != nil {
		var certificateErr rerrors.CertificateResolutionError
		if !errors.As(err, &certificateErr) {
			return err
		}
		l.status.ResolvedRefs.InvalidCertificateRef = certificateErr
		return nil
	}

	l.tls.Certificates = []string{resource}

	if l.listener.TLS.Options != nil {
		tlsMinVersion := l.listener.TLS.Options[tlsMinVersionAnnotationKey]
		tlsMaxVersion := l.listener.TLS.Options[tlsMaxVersionAnnotationKey]
		tlsCipherSuitesStr := l.listener.TLS.Options[tlsCipherSuitesAnnotationKey]

		if tlsMinVersion != "" {
			if _, ok := supportedTlsVersions[string(tlsMinVersion)]; !ok {
				l.status.Ready.Invalid = errors.New("unrecognized TLS min version")
				return nil
			}

			if tlsCipherSuitesStr != "" {
				if _, ok := tlsVersionsWithConfigurableCipherSuites[string(tlsMinVersion)]; !ok {
					l.status.Ready.Invalid = errors.New("configuring TLS cipher suites is only supported for TLS 1.2 and earlier")
					return nil
				}
			}

			l.tls.MinVersion = string(tlsMinVersion)
		}

		if tlsMaxVersion != "" {
			if _, ok := supportedTlsVersions[string(tlsMaxVersion)]; !ok {
				l.status.Ready.Invalid = errors.New("unrecognized TLS max version")
				return nil
			}

			l.tls.MaxVersion = string(tlsMaxVersion)
		}

		if tlsCipherSuitesStr != "" {
			// split comma delimited string into string array and trim whitespace
			tlsCipherSuitesUntrimmed := strings.Split(string(tlsCipherSuitesStr), ",")
			tlsCipherSuites := tlsCipherSuitesUntrimmed[:0]
			for _, c := range tlsCipherSuitesUntrimmed {
				tlsCipherSuites = append(tlsCipherSuites, strings.TrimSpace(c))
			}

			// validate each cipher suite in array
			for _, c := range tlsCipherSuites {
				if ok := common.SupportedTLSCipherSuite(c); !ok {
					l.status.Ready.Invalid = fmt.Errorf("unrecognized or unsupported TLS cipher suite: %s", c)
					return nil
				}
			}

			// set cipher suites on listener TLS params
			l.tls.CipherSuites = tlsCipherSuites
		}
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
	supportedKinds := rcommon.SupportedKindsFor(l.listener.Protocol)
	if supportedKinds == nil {
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

func kindsNotInSet(set, parent []gwv1beta1.RouteGroupKind) []gwv1beta1.RouteGroupKind {
	kinds := []gwv1beta1.RouteGroupKind{}
	for _, kind := range set {
		if !isKindInSet(kind, parent) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func isKindInSet(value gwv1beta1.RouteGroupKind, set []gwv1beta1.RouteGroupKind) bool {
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

func (l *K8sListener) resolveCertificateReference(ctx context.Context, ref gwv1beta1.SecretObjectReference) (string, error) {
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
			return "", rerrors.NewCertificateResolutionErrorNotFound("certificate not found")
		}
		return utils.NewK8sSecret(namespace, string(ref.Name)).String(), nil
	// add more supported types here
	default:
		return "", rerrors.NewCertificateResolutionErrorUnsupported(fmt.Sprintf("unsupported certificate type - group: %s, kind: %s", group, kind))
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

	// Update listener TLS config to specify whether TLS is required by the protocol
	l.tls.Enabled = tls

	return store.ListenerConfig{
		Name:     name,
		Hostname: hostname,
		Port:     int(l.listener.Port),
		Protocol: protocol,
		TLS:      l.tls,
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

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		l.logger.Trace("checking route parent ref", "name", ref.Name)
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			expected := utils.NamespacedName(l.gateway)
			l.logger.Trace("checking gateway match", "expected", expected.String(), "found", namespacedName.String())
			if expected == namespacedName {
				canBind, err := l.canBind(ctx, ref, k8sRoute)
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

func (l *K8sListener) canBind(ctx context.Context, ref gwv1alpha2.ParentReference, route *K8sRoute) (bool, error) {
	if l.status.Ready.HasError() {
		l.logger.Trace("listener not ready, unable to bind", "route", route.ID())
		return false, nil
	}

	l.logger.Trace("checking listener match", "expected", l.listener.Name, "found", ref.SectionName)

	// must is only true if there's a ref with a specific listener name
	// meaning if we must attach, but cannot, it's an error
	allowed, must := routeMatchesListener(l.listener.Name, ref.SectionName)
	if allowed {
		if !routeKindIsAllowedForListener(l.supportedKinds, route) {
			l.logger.Trace("route kind not allowed for listener", "route", route.ID())
			if must {
				return false, rerrors.NewBindErrorRouteKind("route kind not allowed for listener")
			}
			return false, nil
		}

		allowed, err := routeAllowedForListenerNamespaces(ctx, l.gateway.Namespace, l.listener.AllowedRoutes, route, l.client)
		if err != nil {
			return false, fmt.Errorf("error checking listener namespaces: %w", err)
		}
		if !allowed {
			l.logger.Trace("route not allowed because of listener namespace policy", "route", route.ID())
			if must {
				return false, rerrors.NewBindErrorListenerNamespacePolicy("route not allowed because of listener namespace policy")
			}
			return false, nil
		}

		if !route.MatchesHostname(l.listener.Hostname) {
			l.logger.Trace("route does not match listener hostname", "route", route.ID())
			if must {
				return false, rerrors.NewBindErrorHostnameMismatch("route does not match listener hostname")
			}
			return false, nil
		}

		// check if the route is valid, if not, then return a status about it being rejected
		if !route.IsValid() {
			return false, rerrors.NewBindErrorRouteInvalid("route is in an invalid state and cannot bind")
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

func (l *K8sListener) Status() gwv1beta1.ListenerStatus {
	routeCount := atomic.LoadInt32(&l.routeCount)
	if l.listener.Protocol == gwv1beta1.TCPProtocolType {
		if routeCount > 1 {
			l.status.Conflicted.RouteConflict = errors.New("only a single TCP route can be bound to a TCP listener")
		} else {
			l.status.Conflicted.RouteConflict = nil
		}
	}
	return gwv1beta1.ListenerStatus{
		Name:           l.listener.Name,
		SupportedKinds: l.supportedKinds,
		AttachedRoutes: routeCount,
		Conditions:     l.status.Conditions(l.gateway.Generation),
	}
}

func (l *K8sListener) IsValid() bool {
	routeCount := atomic.LoadInt32(&l.routeCount)
	if l.listener.Protocol == gwv1beta1.TCPProtocolType {
		if routeCount > 1 {
			return false
		}
	}
	return l.status.Valid()
}
