package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	rcommon "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
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
	canBind, err = listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
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
	canBind, err = listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
	canBind, err = listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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

	canBind, err := listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.UDPRoute{
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
	_, err = listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.TCPRoute{
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
	_, err := listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
	canBind, err := listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
	_, err = listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
	_, err = listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
	_, err := listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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

	canBind, err := listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
	canBind, err := listener.CanBind(context.Background(), newK8sRoute(&gwv1alpha2.HTTPRoute{
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
