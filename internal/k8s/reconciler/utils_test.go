package reconciler

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
)

func TestRouteMatchesListener(t *testing.T) {
	t.Parallel()

	listenerName := gwv1beta1.SectionName("name")
	routeSectionName := gwv1alpha2.SectionName("name")
	can, must := routeMatchesListener(listenerName, &routeSectionName)
	require.True(t, can)
	require.True(t, must)

	can, must = routeMatchesListener(listenerName, nil)
	require.True(t, can)
	require.False(t, must)

	can, must = routeMatchesListener(gwv1beta1.SectionName("other"), &routeSectionName)
	require.False(t, can)
	require.True(t, must)
}

func TestRouteMatchesListenerHostname(t *testing.T) {
	t.Parallel()

	hostname := gwv1beta1.Hostname("name")
	require.True(t, routeMatchesListenerHostname(nil, nil))
	require.True(t, routeMatchesListenerHostname(&hostname, nil))
	require.True(t, routeMatchesListenerHostname(&hostname, []gwv1alpha2.Hostname{"*"}))
	require.False(t, routeMatchesListenerHostname(&hostname, []gwv1alpha2.Hostname{"other"}))
}

func TestHostnamesMatch(t *testing.T) {
	t.Parallel()

	require.True(t, hostnamesMatch("*", "*"))
	require.True(t, hostnamesMatch("", "*"))
	require.True(t, hostnamesMatch("*", ""))
	require.True(t, hostnamesMatch("", ""))
	require.True(t, hostnamesMatch("*.test", "*.test"))
	require.True(t, hostnamesMatch("a.test", "*.test"))
	require.True(t, hostnamesMatch("*.test", "a.test"))
	require.False(t, hostnamesMatch("*.test", "a.b.test"))
	require.False(t, hostnamesMatch("a.b.test", "*.test"))
	require.True(t, hostnamesMatch("a.b.test", "*.b.test"))
	require.True(t, hostnamesMatch("*.b.test", "a.b.test"))
	require.False(t, hostnamesMatch("*.b.test", "a.c.test"))
	require.True(t, hostnamesMatch("a.b.test", "a.b.test"))
}

func TestRouteKindIsAllowedForListener(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1alpha2.GroupVersion.Group,
		Version: gwv1alpha2.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})

	require.True(t, routeKindIsAllowedForListener(
		[]gwv1beta1.RouteGroupKind{{
			Group: (*gwv1beta1.Group)(&gwv1alpha2.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		factory.NewRoute(&gwv1alpha2.HTTPRoute{TypeMeta: routeMeta})))

	require.False(t, routeKindIsAllowedForListener(
		[]gwv1beta1.RouteGroupKind{{
			Group: (*gwv1beta1.Group)(&gwv1alpha2.GroupVersion.Group),
			Kind:  "TCPRoute",
		}},
		factory.NewRoute(&gwv1alpha2.HTTPRoute{TypeMeta: routeMeta})))
}

func TestRouteAllowedForListenerNamespaces(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	// same
	same := gwv1beta1.NamespacesFromSame

	allowed, err := routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)

	// all
	all := gwv1beta1.NamespacesFromAll
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &all,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	// selector
	selector := gwv1beta1.NamespacesFromSelector

	matchingNamespace := &core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Labels: map[string]string{
				"label":                       "test",
				"kubernetes.io/metadata.name": "expected",
			}}}
	invalidNamespace := &core.Namespace{ObjectMeta: meta.ObjectMeta{Labels: map[string]string{}}}

	client.EXPECT().GetNamespace(context.Background(), types.NamespacedName{Name: "expected"}).Return(invalidNamespace, nil).Times(1)
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)

	client.EXPECT().GetNamespace(context.Background(), types.NamespacedName{Name: "expected"}).Return(matchingNamespace, nil).Times(1)
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	_, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchExpressions: []meta.LabelSelectorRequirement{{
					Key:      "test",
					Operator: meta.LabelSelectorOperator("invalid"),
				}},
			},
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.Error(t, err)

	// unknown
	unknown := gwv1beta1.FromNamespaces("unknown")
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &unknown,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestConditionEqual(t *testing.T) {
	t.Parallel()

	require.True(t, conditionEqual(meta.Condition{}, meta.Condition{}))
	require.False(t, conditionEqual(meta.Condition{
		Type: "expected",
	}, meta.Condition{
		Type: "other",
	}))
	require.False(t, conditionEqual(meta.Condition{
		Reason: "expected",
	}, meta.Condition{
		Reason: "other",
	}))
	require.False(t, conditionEqual(meta.Condition{
		Message: "expected",
	}, meta.Condition{
		Message: "other",
	}))
	require.False(t, conditionEqual(meta.Condition{
		Status: meta.ConditionFalse,
	}, meta.Condition{
		Status: meta.ConditionTrue,
	}))
	require.False(t, conditionEqual(meta.Condition{
		ObservedGeneration: 1,
	}, meta.Condition{
		ObservedGeneration: 2,
	}))
}

func TestConditionsEqual(t *testing.T) {
	t.Parallel()

	require.True(t, conditionsEqual([]meta.Condition{}, []meta.Condition{}))
	require.False(t, conditionsEqual([]meta.Condition{}, []meta.Condition{{}}))
	require.True(t, conditionsEqual([]meta.Condition{{
		Type: "expected",
	}}, []meta.Condition{{
		Type: "expected",
	}}))
	require.False(t, conditionsEqual([]meta.Condition{{
		Type: "expected",
	}}, []meta.Condition{{
		Type: "other",
	}}))
}

func TestListenerStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, listenerStatusEqual(gwv1beta1.ListenerStatus{}, gwv1beta1.ListenerStatus{}))
	require.False(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		Name: "expected",
	}, gwv1beta1.ListenerStatus{
		Name: "other",
	}))
	require.False(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		AttachedRoutes: 1,
	}, gwv1beta1.ListenerStatus{
		AttachedRoutes: 2,
	}))

	groupOne := gwv1beta1.Group("group")
	kindOne := gwv1beta1.Kind("kind")
	groupTwo := gwv1beta1.Group("group")
	kindTwo := gwv1beta1.Kind("kind")
	require.True(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))

	groupTwo = gwv1beta1.Group("other")
	require.False(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))
}

func TestListenerStatusesEqual(t *testing.T) {
	t.Parallel()

	require.True(t, listenerStatusesEqual([]gwv1beta1.ListenerStatus{}, []gwv1beta1.ListenerStatus{}))
	require.False(t, listenerStatusesEqual([]gwv1beta1.ListenerStatus{}, []gwv1beta1.ListenerStatus{{}}))
	require.False(t, listenerStatusesEqual([]gwv1beta1.ListenerStatus{{
		Name: "expected",
	}}, []gwv1beta1.ListenerStatus{{
		Name: "other",
	}}))
}

func TestParentStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, parentStatusEqual(gwv1alpha2.RouteParentStatus{}, gwv1alpha2.RouteParentStatus{}))
	require.False(t, parentStatusEqual(gwv1alpha2.RouteParentStatus{}, gwv1alpha2.RouteParentStatus{
		ControllerName: "other",
	}))
	require.False(t, parentStatusEqual(gwv1alpha2.RouteParentStatus{}, gwv1alpha2.RouteParentStatus{
		ParentRef: gwv1alpha2.ParentReference{
			Name: "other",
		},
	}))
}

func TestGatewayStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, gatewayStatusEqual(gwv1beta1.GatewayStatus{}, gwv1beta1.GatewayStatus{}))
	require.False(t, gatewayStatusEqual(gwv1beta1.GatewayStatus{}, gwv1beta1.GatewayStatus{
		Conditions: []meta.Condition{{}},
	}))
	require.False(t, gatewayStatusEqual(gwv1beta1.GatewayStatus{}, gwv1beta1.GatewayStatus{
		Listeners: []gwv1beta1.ListenerStatus{{}},
	}))
}
