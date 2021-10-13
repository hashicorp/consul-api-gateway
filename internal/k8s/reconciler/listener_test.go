package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestListenerID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).ID())
	require.Equal(t, "test", NewK8sListener(&gw.Gateway{}, gw.Listener{
		Name: gw.SectionName("test"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).ID())
}

func TestListenerValidate(t *testing.T) {
	t.Parallel()

	expected := errors.New("expected")
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	// protocols
	listener := NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition := listener.status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	condition = listener.status.Detached.Condition(0)
	require.Equal(t, ListenerConditionReasonUnsupportedProtocol, condition.Reason)

	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Kinds: []gw.RouteGroupKind{{
				Kind: gw.Kind("UDPRoute"),
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidRouteKinds, condition.Reason)

	// Addresses
	listener = NewK8sListener(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Addresses: []gw.GatewayAddress{{}},
		},
	}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.Detached.Condition(0)
	require.Equal(t, ListenerConditionReasonUnsupportedAddress, condition.Reason)

	// TLS validations
	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)

	mode := gw.TLSModePassthrough
	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			Mode: &mode,
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)

	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS:      &gw.GatewayTLSConfig{},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRef: &gw.SecretObjectReference{
				Name: "secret",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.True(t, errors.Is(listener.Validate(context.Background()), expected))

	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRef: &gw.SecretObjectReference{
				Name: "secret",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRef: &gw.SecretObjectReference{
				Name: "secret",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.Len(t, listener.Certificates(), 0)
	require.NoError(t, listener.Validate(context.Background()))
	require.Len(t, listener.Certificates(), 1)

	group := gw.Group("group")
	kind := gw.Kind("kind")
	namespace := gw.Namespace("namespace")
	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRef: &gw.SecretObjectReference{
				Namespace: &namespace,
				Group:     &group,
				Kind:      &kind,
				Name:      "secret",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	require.NoError(t, listener.Validate(context.Background()))
	condition = listener.status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)
}

func TestIsKindInSet(t *testing.T) {
	t.Parallel()

	require.False(t, isKindInSet(gw.RouteGroupKind{
		Kind: gw.Kind("test"),
	}, []gw.RouteGroupKind{}))
	require.True(t, isKindInSet(gw.RouteGroupKind{
		Kind: gw.Kind("test"),
	}, []gw.RouteGroupKind{{
		Kind: gw.Kind("test"),
	}}))

	group := gw.Group("group")
	require.True(t, isKindInSet(gw.RouteGroupKind{
		Kind:  gw.Kind("test"),
		Group: &group,
	}, []gw.RouteGroupKind{{
		Kind:  gw.Kind("test"),
		Group: &group,
	}}))
}

func TestListenerConfig(t *testing.T) {
	t.Parallel()

	require.Equal(t, store.ListenerConfig{
		Name: "listener",
		TLS:  false,
	}, NewK8sListener(&gw.Gateway{}, gw.Listener{
		Name: gw.SectionName("listener"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())

	hostname := gw.Hostname("hostname")
	require.Equal(t, store.ListenerConfig{
		Name:     "default",
		Hostname: "hostname",
		TLS:      false,
	}, NewK8sListener(&gw.Gateway{}, gw.Listener{
		Hostname: &hostname,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())
}

func TestRouteAddedCallbacks(t *testing.T) {
	t.Parallel()

	listener := NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, int32(0), listener.routeCount)
	listener.OnRouteAdded(nil)
	require.Equal(t, int32(1), listener.routeCount)
	listener.OnRouteRemoved("")
	require.Equal(t, int32(0), listener.routeCount)
}

func TestRouteStatusConditions(t *testing.T) {
	t.Parallel()

	listener := NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Len(t, listener.Status().Conditions, 4)
}
