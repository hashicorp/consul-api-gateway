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
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
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

	t.Run("Unsupported protocol", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
		condition = listener.status.Detached.Condition(0)
		require.Equal(t, ListenerConditionReasonUnsupportedProtocol, condition.Reason)
	})

	t.Run("Invalid route kinds", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPProtocolType,
			AllowedRoutes: &gw.AllowedRoutes{
				Kinds: []gw.RouteGroupKind{{
					Kind: "UDPRoute",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalidRouteKinds, condition.Reason)
	})

	t.Run("Unsupported address", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{
			Spec: gw.GatewaySpec{
				Addresses: []gw.GatewayAddress{{}},
			},
		}, gw.Listener{
			Protocol: gw.HTTPProtocolType,
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Detached.Condition(0)
		require.Equal(t, ListenerConditionReasonUnsupportedAddress, condition.Reason)
	})

	t.Run("Invalid TLS config", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	})

	t.Run("Invalid TLS passthrough", func(t *testing.T) {
		mode := gw.TLSModePassthrough
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				Mode: &mode,
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
	})

	t.Run("Invalid certificate ref", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS:      &gw.GatewayTLSConfig{},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Fail to retrieve secret", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		require.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Invalid cross-namespace secret ref with no ReferencePolicy", func(t *testing.T) {
		otherNamespace := gw.Namespace("other-namespace")
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
					Namespace: &otherNamespace,
					Name:      "secret",
				}},
			},
		}, K8sListenerConfig{
			Logger: hclog.NewNullLogger(),
			Client: client,
		})
		client.EXPECT().GetReferencePoliciesInNamespace(gomock.Any(), string(otherNamespace)).Return([]gw.ReferencePolicy{}, nil)
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.ResolvedRefs.Condition(0)
		assert.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Valid cross-namespace secret ref with ReferencePolicy", func(t *testing.T) {
		gatewayNamespace := gw.Namespace("gateway-namespace")
		secretNamespace := gw.Namespace("secret-namespace")
		listener := NewK8sListener(
			&gw.Gateway{
				ObjectMeta: meta.ObjectMeta{Namespace: string(gatewayNamespace)},
				TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "Gateway"}},
			gw.Listener{
				Protocol: gw.HTTPSProtocolType,
				TLS: &gw.GatewayTLSConfig{
					CertificateRefs: []gw.SecretObjectReference{{
						Namespace: &secretNamespace,
						Name:      "secret",
					}},
				},
			}, K8sListenerConfig{
				Logger: hclog.NewNullLogger(),
				Client: client,
			})
		client.EXPECT().GetReferencePoliciesInNamespace(gomock.Any(), string(secretNamespace)).
			Return([]gw.ReferencePolicy{{
				Spec: gw.ReferencePolicySpec{
					From: []gw.ReferencePolicyFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Namespace: gatewayNamespace,
					}},
					To: []gw.ReferencePolicyTo{{
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

	t.Run("Valid same-namespace secret ref without ReferencePolicy", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		group := gw.Group("group")
		kind := gw.Kind("kind")
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		assert.Equal(t, ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Valid minimum TLS version", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonReady, condition.Reason)
		require.Equal(t, "TLSv1_2", listener.tls.MinVersion)
	})

	t.Run("Invalid minimum TLS version", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
		require.Equal(t, "unrecognized TLS min version", condition.Message)
	})

	t.Run("Valid TLS cipher suite", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonReady, condition.Reason)
		require.Equal(t, []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, listener.tls.CipherSuites)
	})

	t.Run("TLS cipher suite not allowed", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
		require.Equal(t, "configuring TLS cipher suites is only supported for TLS 1.2 and earlier", condition.Message)
	})

	t.Run("Invalid TLS cipher suite", func(t *testing.T) {
		listener := NewK8sListener(&gw.Gateway{}, gw.Listener{
			Protocol: gw.HTTPSProtocolType,
			TLS: &gw.GatewayTLSConfig{
				CertificateRefs: []gw.SecretObjectReference{{
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
		require.NoError(t, listener.Validate(context.Background()))
		condition := listener.status.Ready.Condition(0)
		require.Equal(t, ListenerConditionReasonInvalid, condition.Reason)
		require.Equal(t, "unrecognized or unsupported TLS cipher suite: foo", condition.Message)
	})
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
	}, NewK8sListener(&gw.Gateway{}, gw.Listener{
		Name: gw.SectionName("listener"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())

	hostname := gw.Hostname("hostname")
	require.Equal(t, store.ListenerConfig{
		Name:     "default",
		Hostname: "hostname",
		TLS:      core.TLSParams{Enabled: false},
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

func TestListenerStatusConditions(t *testing.T) {
	t.Parallel()

	listener := NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Len(t, listener.Status().Conditions, 4)
}

func TestListenerCanBind(t *testing.T) {
	t.Parallel()

	// alternative type
	listener := NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err := listener.CanBind(context.Background(), storeMocks.NewMockRoute(nil))
	require.NoError(t, err)
	require.False(t, canBind)

	// no match
	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err = listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)

	// match
	listener = NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err = listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
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
	listener = NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.status.Ready.Invalid = errors.New("invalid")
	canBind, err = listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
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
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "UDPRoute",
	})
	name := gw.SectionName("listener")

	listener := NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.NoError(t, listener.Validate(context.Background()))
	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gw.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name: "gateway",
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)

	listener = NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = supportedProtocols[gw.HTTPProtocolType]
	_, err = listener.CanBind(context.Background(), NewK8sRoute(&gw.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name:        "gateway",
					SectionName: &name,
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

	name := gw.SectionName("listener")
	same := gw.NamespacesFromSame
	selector := gw.NamespacesFromSelector
	other := gw.Namespace("other")
	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})

	listener := NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "other",
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Namespaces: &gw.RouteNamespaces{
				From: &same,
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = supportedProtocols[gw.HTTPProtocolType]
	_, err := listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name:        "gateway",
					Namespace:   &other,
					SectionName: &name,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)
	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
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

	listener = NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "other",
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Namespaces: &gw.RouteNamespaces{
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
	listener.supportedKinds = supportedProtocols[gw.HTTPProtocolType]
	_, err = listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name:        "gateway",
					Namespace:   &other,
					SectionName: &name,
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)
	_, err = listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
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
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	name := gw.SectionName("listener")
	hostname := gw.Hostname("hostname")
	listener := NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gw.Listener{
		Name:     name,
		Hostname: &hostname,
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.supportedKinds = supportedProtocols[gw.HTTPProtocolType]
	_, err := listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name:        "gateway",
					SectionName: &name,
				}},
			},
			Hostnames: []gw.Hostname{"other"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)

	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name: "gateway",
				}},
			},
			Hostnames: []gw.Hostname{"other"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)
}

func TestListenerCanBind_NameMatch(t *testing.T) {
	t.Parallel()

	name := gw.SectionName("listener")
	otherName := gw.SectionName("other")
	listener := NewK8sListener(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "gateway",
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err := listener.CanBind(context.Background(), NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentReference{{
					Name:        "gateway",
					SectionName: &otherName,
				}},
			},
			Hostnames: []gw.Hostname{"other"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, canBind)
}
