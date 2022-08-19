package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8s "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	rcommon "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
)

func TestListenerID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).ID())
	require.Equal(t, "test", NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
		Name: gwv1beta1.SectionName("test"),
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

	t.Run("Unsupported protocol", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalid, condition.Reason)
		condition = listener.status.Detached.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonUnsupportedProtocol, condition.Reason)
	})

	t.Run("Invalid route kinds", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPProtocolType,
			AllowedRoutes: &gwv1beta1.AllowedRoutes{
				Kinds: []gwv1beta1.RouteGroupKind{{
					Kind: "UDPRoute",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalidRouteKinds, condition.Reason)
	})

	t.Run("Unsupported address", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{
			Spec: gwv1beta1.GatewaySpec{
				Addresses: []gwv1beta1.GatewayAddress{{}},
			},
		}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPProtocolType,
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Detached.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonUnsupportedAddress, condition.Reason)
	})

	t.Run("Invalid TLS config", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalid, condition.Reason)
	})

	t.Run("Invalid TLS passthrough", func(t *testing.T) {
		mode := gwv1beta1.TLSModePassthrough
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				Mode: &mode,
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalid, condition.Reason)
	})

	t.Run("Invalid certificate ref", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS:      &gwv1beta1.GatewayTLSConfig{},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Fail to retrieve secret", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
			Client: client,
		})
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
		require.True(t, errors.Is(listener.Validate(context.Background()), expected))
	})

	t.Run("No secret found", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
			Client: client,
		})
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Invalid cross-namespace secret ref with no ReferenceGrant", func(t *testing.T) {
		otherNamespace := gwv1beta1.Namespace("other-namespace")
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Namespace: &otherNamespace,
					Name:      "secret",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
			Client: client,
		})
		client.EXPECT().GetReferenceGrantsInNamespace(gomock.Any(), string(otherNamespace)).Return([]gwv1alpha2.ReferenceGrant{}, nil)
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		assert.Equal(t, rstatus.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Valid cross-namespace secret ref with ReferenceGrant", func(t *testing.T) {
		gatewayNamespace := gwv1beta1.Namespace("gateway-namespace")
		refGrantGatewayNamespace := gwv1alpha2.Namespace("gateway-namespace")
		secretNamespace := gwv1beta1.Namespace("secret-namespace")
		listener := NewK8sListener(
			&gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{Namespace: string(gatewayNamespace)},
				TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "Gateway"}},
			gwv1beta1.Listener{
				Protocol: gwv1beta1.HTTPSProtocolType,
				TLS: &gwv1beta1.GatewayTLSConfig{
					CertificateRefs: []gwv1beta1.SecretObjectReference{{
						Namespace: &secretNamespace,
						Name:      "secret",
					}},
				},
			}, K8sListenerConfig{
				Logger: hclog.NewNullLogger(),
				Client: client,
			})
		client.EXPECT().GetReferenceGrantsInNamespace(gomock.Any(), string(secretNamespace)).
			Return([]gwv1alpha2.ReferenceGrant{{
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Namespace: refGrantGatewayNamespace,
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			}}, nil)
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&k8s.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		assert.Equal(t, meta.ConditionTrue, condition.Status)
	})

	t.Run("Valid same-namespace secret ref without ReferenceGrant", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
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
		assert.Len(t, listener.Config().TLS.Certificates, 0)
		assert.NoError(t, listener.Validate(context.Background()))
		assert.Len(t, listener.Config().TLS.Certificates, 1)
	})

	t.Run("Unsupported certificate type", func(t *testing.T) {
		group := gwv1beta1.Group("group")
		kind := gwv1beta1.Kind("kind")
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Group: &group,
					Kind:  &kind,
					Name:  "secret",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
			Client: client,
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		assert.Equal(t, rstatus.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Valid minimum TLS version", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
				Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonReady, condition.Reason)
		require.Equal(t, "TLSv1_2", listener.tls.MinVersion)
	})

	t.Run("Invalid minimum TLS version", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
				Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalid, condition.Reason)
		require.Equal(t, "unrecognized TLS min version", condition.Message)
	})

	t.Run("Valid TLS cipher suite", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
				Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonReady, condition.Reason)
		require.Equal(t, []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, listener.tls.CipherSuites)
	})

	t.Run("TLS cipher suite not allowed", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
				Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalid, condition.Reason)
		require.Equal(t, "configuring TLS cipher suites is only supported for TLS 1.2 and earlier", condition.Message)
	})

	t.Run("Invalid TLS cipher suite", func(t *testing.T) {
		listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
				Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, rstatus.ListenerConditionReasonInvalid, condition.Reason)
		require.Equal(t, "unrecognized or unsupported TLS cipher suite: foo", condition.Message)
	})
}

func TestIsKindInSet(t *testing.T) {
	t.Parallel()

	require.False(t, isKindInSet(gwv1beta1.RouteGroupKind{
		Kind: gwv1beta1.Kind("test"),
	}, []gwv1beta1.RouteGroupKind{}))
	require.True(t, isKindInSet(gwv1beta1.RouteGroupKind{
		Kind: gwv1beta1.Kind("test"),
	}, []gwv1beta1.RouteGroupKind{{
		Kind: gwv1beta1.Kind("test"),
	}}))

	group := gwv1beta1.Group("group")
	require.True(t, isKindInSet(gwv1beta1.RouteGroupKind{
		Kind:  gwv1beta1.Kind("test"),
		Group: &group,
	}, []gwv1beta1.RouteGroupKind{{
		Kind:  gwv1beta1.Kind("test"),
		Group: &group,
	}}))
}

func TestListenerConfig(t *testing.T) {
	t.Parallel()

	require.Equal(t, store.ListenerConfig{
		Name: "listener",
		TLS:  core.TLSParams{Enabled: false},
	}, NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
		Name: gwv1beta1.SectionName("listener"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())

	hostname := gwv1beta1.Hostname("hostname")
	require.Equal(t, store.ListenerConfig{
		Name:     "default",
		Hostname: "hostname",
		TLS:      core.TLSParams{Enabled: false},
	}, NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{
		Hostname: &hostname,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())
}

func TestRouteAddedCallbacks(t *testing.T) {
	t.Parallel()

	listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, int32(0), listener.routeCount)
	listener.OnRouteAdded(nil)
	require.Equal(t, int32(1), listener.routeCount)
	listener.OnRouteRemoved("")
	require.Equal(t, int32(0), listener.routeCount)
}

func TestListenerStatusConditions(t *testing.T) {
	t.Parallel()

	listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Len(t, listener.Status().Conditions, 4)
}

func TestListenerCanBind(t *testing.T) {
	t.Parallel()

	// alternative type
	listener := NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err := listener.CanBind(context.Background(), storeMocks.NewMockRoute(nil))
	require.NoError(t, err)
	require.False(t, canBind)

	// no match
	listener = NewK8sListener(&gwv1beta1.Gateway{}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err = listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)

	// match
	listener = NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err = listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "gateway",
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.True(t, canBind)

	// not ready
	listener = NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gwv1beta1.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.status.Ready.Invalid = errors.New("invalid")
	canBind, err = listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "gateway",
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)
}

func TestListenerCanBind_RouteKind(t *testing.T) {
	t.Parallel()

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1alpha2.GroupVersion.Group,
		Version: gwv1alpha2.GroupVersion.Version,
		Kind:    "TCPRoute",
	})
	listenerName := gwv1beta1.SectionName("listener")
	routeSectionName := gwv1alpha2.SectionName("listener")

	listener := NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))

	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.UDPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "gateway",
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)

	listener = NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gwv1beta1.Listener{
		Name:     listenerName,
		Protocol: gwv1beta1.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = rcommon.SupportedKindsFor(gwv1beta1.HTTPProtocolType)
	_, err = listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.TCPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:        "gateway",
					SectionName: &routeSectionName,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)
}

func TestListenerCanBind_AllowedNamespaces(t *testing.T) {
	t.Parallel()

	listenerName := gwv1beta1.SectionName("listener")
	routeSectionName := gwv1alpha2.SectionName("listener")
	same := gwv1beta1.NamespacesFromSame
	selector := gwv1beta1.NamespacesFromSelector
	other := gwv1alpha2.Namespace("other")
	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1beta1.GroupVersion.Group,
		Version: gwv1beta1.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})

	listener := NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "other",
		},
	}, gwv1beta1.Listener{
		Name:     listenerName,
		Protocol: gwv1beta1.HTTPProtocolType,
		AllowedRoutes: &gwv1beta1.AllowedRoutes{
			Namespaces: &gwv1beta1.RouteNamespaces{
				From: &same,
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = rcommon.SupportedKindsFor(gwv1beta1.HTTPProtocolType)
	_, err := listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:        "gateway",
					Namespace:   &other,
					SectionName: &routeSectionName,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)
	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:      "gateway",
					Namespace: &other,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)

	listener = NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "other",
		},
	}, gwv1beta1.Listener{
		Name:     listenerName,
		Protocol: gwv1beta1.HTTPProtocolType,
		AllowedRoutes: &gwv1beta1.AllowedRoutes{
			Namespaces: &gwv1beta1.RouteNamespaces{
				From: &selector,
				Selector: &meta.LabelSelector{
					MatchExpressions: []meta.LabelSelectorRequirement{{
						Key:      "test",
						Operator: meta.LabelSelectorOperator("invalid"),
					}},
				},
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = rcommon.SupportedKindsFor(gwv1beta1.HTTPProtocolType)
	_, err = listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:        "gateway",
					Namespace:   &other,
					SectionName: &routeSectionName,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)
	_, err = listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:      "gateway",
					Namespace: &other,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)
}

func TestListenerCanBind_HostnameMatch(t *testing.T) {
	t.Parallel()

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1beta1.GroupVersion.Group,
		Version: gwv1beta1.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	listenerName := gwv1beta1.SectionName("listener")
	routeSectionName := gwv1alpha2.SectionName("listener")
	hostname := gwv1beta1.Hostname("hostname")
	listener := NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gwv1beta1.Listener{
		Name:     listenerName,
		Hostname: &hostname,
		Protocol: gwv1beta1.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = rcommon.SupportedKindsFor(gwv1beta1.HTTPProtocolType)
	_, err := listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:        "gateway",
					SectionName: &routeSectionName,
				}},
			},
			Hostnames: []gwv1alpha2.Hostname{"other"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)

	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "gateway",
				}},
			},
			Hostnames: []gwv1alpha2.Hostname{"other"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)
}

func TestListenerCanBind_NameMatch(t *testing.T) {
	t.Parallel()

	listenerName := gwv1beta1.SectionName("listener")
	otherName := gwv1alpha2.SectionName("other")
	listener := NewK8sListener(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gwv1beta1.Listener{
		Name:     listenerName,
		Protocol: gwv1beta1.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:        "gateway",
					SectionName: &otherName,
				}},
			},
			Hostnames: []gwv1alpha2.Hostname{"other"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)
}
