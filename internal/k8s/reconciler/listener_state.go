package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// ListenerState holds ephemeral state for listeners
type ListenerState struct {
	RouteCount int32
	TLS        core.TLSParams
	Status     ListenerStatus
}

func (l *ListenerState) Validate(ctx context.Context, client gatewayclient.Client, gateway *gw.Gateway, listener gw.Listener) error {
	l.validateUnsupported(gateway)
	l.validateProtocols(listener)

	if err := l.validateTLS(ctx, client, gateway, listener); err != nil {
		return err
	}

	if l.Status.Ready.Invalid == nil && !l.Status.Valid() {
		// set the listener as invalid if any other statuses are not valid
		l.Status.Ready.Invalid = errors.New("listener is in an invalid state")
	}

	return nil
}

func (l *ListenerState) validateUnsupported(gateway *gw.Gateway) {
	// seems weird that we're looking at gateway fields for listener status
	// but that's the weirdness of the spec
	if len(gateway.Spec.Addresses) > 0 {
		// we dnn't support address binding
		l.Status.Detached.UnsupportedAddress = errors.New("specified addresses are not supported")
	}
}

func (l *ListenerState) validateProtocols(listener gw.Listener) {
	supportedKinds := supportedKindsFor(listener.Protocol)
	if len(supportedKinds) == 0 {
		l.Status.Detached.UnsupportedProtocol = fmt.Errorf("unsupported protocol: %s", listener.Protocol)
	}
	if listener.AllowedRoutes != nil {
		remainderKinds := kindsNotInSet(listener.AllowedRoutes.Kinds, supportedKinds)
		if len(remainderKinds) != 0 {
			l.Status.ResolvedRefs.InvalidRouteKinds = fmt.Errorf("listener has unsupported kinds: %v", remainderKinds)
		}
	}
}

func (l *ListenerState) validateTLS(ctx context.Context, client gatewayclient.Client, gateway *gw.Gateway, listener gw.Listener) error {
	_, tlsRequired := utils.ProtocolToConsul(listener.Protocol)
	if listener.TLS == nil {
		// TODO: should this struct field be "Required" instead of "Enabled"?
		if tlsRequired {
			// we are using a protocol that requires TLS but has no TLS
			// configured
			l.Status.Ready.Invalid = errors.New("tls configuration required for the given protocol")
		}
		return nil
	}

	if listener.TLS.Mode != nil && *listener.TLS.Mode == gw.TLSModePassthrough {
		l.Status.Ready.Invalid = errors.New("tls passthrough not supported")
		return nil
	}

	if len(listener.TLS.CertificateRefs) == 0 {
		l.Status.ResolvedRefs.InvalidCertificateRef = errors.New("certificate reference must be set")
		return nil
	}

	// we only support a single certificate for now
	ref := *listener.TLS.CertificateRefs[0]
	resource, err := resolveCertificateReference(ctx, client, gateway, ref)
	if err != nil {
		var certificateErr CertificateResolutionError
		if !errors.As(err, &certificateErr) {
			return err
		}
		l.Status.ResolvedRefs.InvalidCertificateRef = certificateErr
	} else {
		l.TLS.Certificates = []string{resource}
	}

	if listener.TLS.Options != nil {
		tlsMinVersion := listener.TLS.Options[tlsMinVersionAnnotationKey]
		tlsMaxVersion := listener.TLS.Options[tlsMaxVersionAnnotationKey]
		tlsCipherSuitesStr := listener.TLS.Options[tlsCipherSuitesAnnotationKey]

		if tlsMinVersion != "" {
			if _, ok := supportedTlsVersions[string(tlsMinVersion)]; !ok {
				l.Status.Ready.Invalid = errors.New("unrecognized TLS min version")
				return nil
			}

			if tlsCipherSuitesStr != "" {
				if _, ok := tlsVersionsWithConfigurableCipherSuites[string(tlsMinVersion)]; !ok {
					l.Status.Ready.Invalid = errors.New("configuring TLS cipher suites is only supported for TLS 1.2 and earlier")
					return nil
				}
			}

			l.TLS.MinVersion = string(tlsMinVersion)
		}

		if tlsMaxVersion != "" {
			if _, ok := supportedTlsVersions[string(tlsMaxVersion)]; !ok {
				l.Status.Ready.Invalid = errors.New("unrecognized TLS max version")
				return nil
			}

			l.TLS.MaxVersion = string(tlsMaxVersion)
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
					l.Status.Ready.Invalid = fmt.Errorf("unrecognized or unsupported TLS cipher suite: %s", c)
					return nil
				}
			}

			// set cipher suites on listener TLS params
			l.TLS.CipherSuites = tlsCipherSuites
		}
	}

	return nil
}

func resolveCertificateReference(ctx context.Context, client gatewayclient.Client, gateway *gw.Gateway, ref gw.SecretObjectReference) (string, error) {
	group := corev1.GroupName
	kind := "Secret"
	namespace := gateway.Namespace

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
		cert, err := client.GetSecret(ctx, types.NamespacedName{Name: string(ref.Name), Namespace: namespace})
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

func (l *ListenerState) ValidWithProtocol(protocol gw.ProtocolType) bool {
	routeCount := atomic.LoadInt32(&l.RouteCount)
	if protocol == gw.TCPProtocolType {
		if routeCount > 1 {
			return false
		}
	}
	return l.Status.Valid()
}

func (l *ListenerState) GetStatus(listener gw.Listener, generation int64) gw.ListenerStatus {
	routeCount := atomic.LoadInt32(&l.RouteCount)
	if listener.Protocol == gw.TCPProtocolType {
		if routeCount > 1 {
			l.Status.Conflicted.RouteConflict = errors.New("only a single TCP route can be bound to a TCP listener")
		} else {
			l.Status.Conflicted.RouteConflict = nil
		}
	}
	return gw.ListenerStatus{
		Name:           listener.Name,
		SupportedKinds: supportedKindsFor(listener.Protocol),
		AttachedRoutes: routeCount,
		Conditions:     l.Status.Conditions(generation),
	}
}
