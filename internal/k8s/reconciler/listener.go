package reconciler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/go-hclog"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	ErrInvalidGatewayListener    = errors.New("invalid gateway listener")
	ErrTLSPassthroughUnsupported = errors.New("tls passthrough unsupported")
	ErrInvalidTLSConfiguration   = errors.New("invalid tls configuration")
	ErrInvalidTLSCertReference   = errors.New("invalid tls certificate reference")
	ErrCannotBindListener        = errors.New("cannot bind listener")
)

const (
	defaultListenerName = "default"

	ConditionReasonUnableToBind  = "UnableToBindGateway"
	ConditionReasonRouteAdmitted = "RouteAdmitted"
)

type resolvedCertificate struct {
	err         error
	certificate string
}

type K8sListener struct {
	consulNamespace string
	logger          hclog.Logger
	gateway         *gw.Gateway
	listener        gw.Listener
	client          gatewayclient.Client

	routeCount      int32
	resolutionError error
	certificates    []resolvedCertificate
}

var _ core.RouteTrackingListener = &K8sListener{}

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

func (l *K8sListener) Logger() hclog.Logger {
	return l.logger
}

func (l *K8sListener) Certificates() []string {
	certificates := []string{}
	for _, cert := range l.certificates {
		if cert.certificate != "" {
			certificates = append(certificates, cert.certificate)
		}
	}
	return certificates
}

func (l *K8sListener) ResolveCertificates(ctx context.Context) error {
	if l.listener.TLS == nil {
		return nil
	}

	if l.listener.TLS.Mode != nil && *l.listener.TLS.Mode == gw.TLSModePassthrough {
		l.resolutionError = ErrTLSPassthroughUnsupported
		return nil
	}

	if l.listener.TLS.CertificateRef == nil {
		l.resolutionError = ErrInvalidTLSCertReference
		return nil
	}

	ref := *l.listener.TLS.CertificateRef
	resource, err := l.resolveCertificateReference(ctx, ref)
	if err != nil {
		if !errors.Is(err, ErrInvalidTLSCertReference) {
			return err
		}
		l.certificates = []resolvedCertificate{{
			err: err,
		}}
	} else {
		l.certificates = []resolvedCertificate{{
			certificate: resource,
		}}
	}

	return nil
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

func (l *K8sListener) Config() core.ListenerConfig {
	name := defaultListenerName
	if l.listener.Name != "" {
		name = string(l.listener.Name)
	}
	hostname := ""
	if l.listener.Hostname != nil {
		hostname = string(*l.listener.Hostname)
	}
	protocol, tls := utils.ProtocolToConsul(l.listener.Protocol)
	return core.ListenerConfig{
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
func (l *K8sListener) CanBind(route core.Route) (bool, error) {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false, nil
	}

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := referencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(l.gateway) == namespacedName {
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
	if l.resolutionError != nil {
		return false, nil
	}

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

		return true, nil
	}

	return false, nil
}

func (l *K8sListener) OnRouteAdded(_ core.Route) {
	atomic.AddInt32(&l.routeCount, 1)
}

func (l *K8sListener) OnRouteRemoved(_ core.Route) {
	atomic.AddInt32(&l.routeCount, -1)
}

func (l *K8sListener) status() gw.ListenerStatus {
	certificateRefCondition := meta.Condition{
		Type:               string(gw.ListenerConditionResolvedRefs),
		ObservedGeneration: l.gateway.Generation,
		LastTransitionTime: meta.Now(),
	}
	if l.resolutionError != nil {
		certificateRefCondition.Status = meta.ConditionFalse
		certificateRefCondition.Reason = string(gw.ListenerReasonInvalidCertificateRef)
		certificateRefCondition.Message = l.resolutionError.Error()
	} else {
		// TODO: handle other resolved refs issues
		certificateRefCondition.Status = meta.ConditionTrue
		certificateRefCondition.Reason = string(gw.ListenerReasonResolvedRefs)
	}
	return gw.ListenerStatus{
		Name:           l.listener.Name,
		SupportedKinds: allowedKinds(l.listener.AllowedRoutes.Kinds),
		AttachedRoutes: atomic.LoadInt32(&l.routeCount),
		Conditions:     []meta.Condition{certificateRefCondition},
	}
}

func allowedKinds(allowed []gw.RouteGroupKind) []gw.RouteGroupKind {
	if allowed != nil {
		return allowed
	}
	return []gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "HTTPRoute",
	}}
}

func conditionsEqual(a, b []meta.Condition) bool {
	if len(a) != len(b) {
		// we have a different number of conditions, so they aren't the same
		return false
	}

	for i, newCondition := range a {
		oldCondition := b[i]
		if newCondition.Type != oldCondition.Type ||
			newCondition.Status != oldCondition.Status ||
			newCondition.Reason != oldCondition.Reason ||
			newCondition.Message != oldCondition.Message {
			return false
		}
	}
	return true
}

func listenerStatusEqual(a, b gw.ListenerStatus) bool {
	if a.Name != b.Name {
		return false
	}
	if !reflect.DeepEqual(a.SupportedKinds, b.SupportedKinds) {
		return false
	}
	if a.AttachedRoutes != b.AttachedRoutes {
		return false
	}
	return conditionsEqual(a.Conditions, b.Conditions)
}

func listenerStatusesEqual(a, b []gw.ListenerStatus) bool {
	if len(a) != len(b) {
		// we have a different number of conditions, so they aren't the same
		return false
	}
	for i, newStatus := range a {
		if !listenerStatusEqual(newStatus, b[i]) {
			return false
		}
	}
	return true
}
