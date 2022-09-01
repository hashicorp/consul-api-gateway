package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	rerrors "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

const (
	defaultListenerName = "default"
)

type K8sListener struct {
	ListenerState *state.ListenerState

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
	State           *state.ListenerState
}

func NewK8sListener(gateway *gwv1beta1.Gateway, listener gwv1beta1.Listener, config K8sListenerConfig) *K8sListener {
	listenerLogger := config.Logger.Named("listener").With("listener", string(listener.Name))

	lState := config.State
	if lState == nil {
		lState = &state.ListenerState{
			Name:     listener.Name,
			Protocol: listener.Protocol,
			Routes:   make(map[string]core.ResolvedRoute),
		}
	}

	return &K8sListener{
		ListenerState:   lState,
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

	// Update listener TLS config to specify whether TLS is required by the protocol
	protocol, tls := utils.ProtocolToConsul(l.ListenerState.Protocol)
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

		if !route.matchesHostname(l.listener.Hostname) {
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

func (l *K8sListener) OnRouteAdded(route store.Route) {
	atomic.AddInt32(&l.routeCount, 1)

	if k8sRoute, ok := route.(*K8sRoute); ok {
		l.ListenerState.Routes[route.ID()] = k8sRoute.resolve(l.consulNamespace, l.gateway, l.listener)
	}
}

func (l *K8sListener) OnRouteRemoved(routeID string) {
	atomic.AddInt32(&l.routeCount, -1)
	delete(l.ListenerState.Routes, routeID)
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
		SupportedKinds: common.SupportedKindsFor(l.listener.Protocol),
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
