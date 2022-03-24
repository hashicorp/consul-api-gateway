package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	k8s "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
)

func TestListenerID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).ID())
	require.Equal(t, "test", NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
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
	listener := NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition := listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	condition = listener.ListenerState.Status.Detached.Condition(0)
	require.Equal(t, ListenerConditionReasonUnsupportedProtocol, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Kinds: []gw.RouteGroupKind{{
				Kind: gw.Kind("UDPRoute"),
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidRouteKinds, condition.Reason)

	// Addresses
	listener = NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Addresses: []gw.GatewayAddress{{}},
			},
		},
	}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Detached.Condition(0)
	require.Equal(t, ListenerConditionReasonUnsupportedAddress, condition.Reason)

	// TLS validations
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)

	mode := gw.TLSModePassthrough
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			Mode: &mode,
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS:      &gw.GatewayTLSConfig{},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.True(t, errors.Is(listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener), expected))

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.Len(t, listener.Config().TLS.Certificates, 0)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	require.Len(t, listener.Config().TLS.Certificates, 1)

	group := gw.Group("group")
	kind := gw.Kind("kind")
	namespace := gw.Namespace("namespace")
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Namespace: &namespace,
				Group:     &group,
				Kind:      &kind,
				Name:      "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version": "TLSv1_2",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonReady, condition.Reason)
	require.Equal(t, "TLSv1_2", listener.ListenerState.TLS.MinVersion)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version": "foo",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "unrecognized TLS min version", condition.Message)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonReady, condition.Reason)
	require.Equal(t, []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, listener.ListenerState.TLS.CipherSuites)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version":   "TLSv1_3",
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "foo",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "configuring TLS cipher suites is only supported for TLS 1.2 and earlier", condition.Message)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, foo",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.NoError(t, listener.Validate(context.Background(), client, listener.gateway.Gateway, listener.listener))
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "unrecognized or unsupported TLS cipher suite: foo", condition.Message)
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
		TLS:  core.TLSParams{Enabled: false},
	}, NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Name: gw.SectionName("listener"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())

	hostname := gw.Hostname("hostname")
	require.Equal(t, store.ListenerConfig{
		Name:     "default",
		Hostname: "hostname",
		TLS:      core.TLSParams{Enabled: false},
	}, NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Hostname: &hostname,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())
}

func TestListenerStatusConditions(t *testing.T) {
	t.Parallel()

	listener := NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Len(t, listener.Status().Conditions, 4)
}
