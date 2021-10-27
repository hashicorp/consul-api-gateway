package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
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
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
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
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
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
	canBind, err := listener.CanBind(storeMocks.NewMockRoute(nil))
	require.NoError(t, err)
	require.False(t, canBind)

	// no match
	listener = NewK8sListener(&gw.Gateway{}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	canBind, err = listener.CanBind(NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
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
	canBind, err = listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	canBind, err = listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	canBind, err := listener.CanBind(NewK8sRoute(&gw.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	_, err = listener.CanBind(NewK8sRoute(&gw.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	_, err := listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	canBind, err := listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	_, err = listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	_, err = listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	_, err := listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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

	canBind, err := listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
	canBind, err := listener.CanBind(NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
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
