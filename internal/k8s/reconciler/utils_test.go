package reconciler

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteMatchesListener(t *testing.T) {
	t.Parallel()

	name := gw.SectionName("name")
	can, must := routeMatchesListener(name, &name)
	require.True(t, can)
	require.True(t, must)

	can, must = routeMatchesListener(name, nil)
	require.True(t, can)
	require.False(t, must)

	can, must = routeMatchesListener(gw.SectionName("other"), &name)
	require.False(t, can)
	require.True(t, must)
}

func TestRouteMatchesListenerHostname(t *testing.T) {
	t.Parallel()

	hostname := gw.Hostname("name")
	require.True(t, routeMatchesListenerHostname(nil, nil))
	require.True(t, routeMatchesListenerHostname(&hostname, nil))
	require.True(t, routeMatchesListenerHostname(&hostname, []gw.Hostname{"*"}))
	require.False(t, routeMatchesListenerHostname(&hostname, []gw.Hostname{"other"}))
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

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	require.True(t, routeKindIsAllowedForListener([]gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "HTTPRoute",
	}}, NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))
	require.False(t, routeKindIsAllowedForListener([]gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "TCPRoute",
	}}, NewK8sRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))
}

func TestRouteAllowedForListenerNamespaces(t *testing.T) {
	t.Parallel()

	// same
	same := gw.NamespacesFromSame
	allowed, err := routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &same,
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &same,
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, allowed)

	// all
	all := gw.NamespacesFromAll
	allowed, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &all,
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.True(t, allowed)

	// selector
	selector := gw.NamespacesFromSelector
	allowed, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.False(t, allowed)

	allowed, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
			Labels: map[string]string{
				"label":                       "test",
				"kubernetes.io/metadata.name": "expected",
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
			Labels: map[string]string{
				"label": "test",
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.NoError(t, err)
	require.True(t, allowed)

	_, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchExpressions: []meta.LabelSelectorRequirement{{
					Key:      "test",
					Operator: meta.LabelSelectorOperator("invalid"),
				}},
			},
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
	require.Error(t, err)

	// unknown
	unknown := gw.FromNamespaces("unknown")
	allowed, err = routeAllowedForListenerNamespaces("expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &unknown,
		},
	}, NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}))
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

	require.True(t, listenerStatusEqual(gw.ListenerStatus{}, gw.ListenerStatus{}))
	require.False(t, listenerStatusEqual(gw.ListenerStatus{
		Name: "expected",
	}, gw.ListenerStatus{
		Name: "other",
	}))
	require.False(t, listenerStatusEqual(gw.ListenerStatus{
		AttachedRoutes: 1,
	}, gw.ListenerStatus{
		AttachedRoutes: 2,
	}))

	groupOne := gw.Group("group")
	kindOne := gw.Kind("kind")
	groupTwo := gw.Group("group")
	kindTwo := gw.Kind("kind")
	require.True(t, listenerStatusEqual(gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))

	groupTwo = gw.Group("other")
	require.False(t, listenerStatusEqual(gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))
}

func TestListenerStatusesEqual(t *testing.T) {
	t.Parallel()

	require.True(t, listenerStatusesEqual([]gw.ListenerStatus{}, []gw.ListenerStatus{}))
	require.False(t, listenerStatusesEqual([]gw.ListenerStatus{}, []gw.ListenerStatus{{}}))
	require.False(t, listenerStatusesEqual([]gw.ListenerStatus{{
		Name: "expected",
	}}, []gw.ListenerStatus{{
		Name: "other",
	}}))
}

func TestParentStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, parentStatusEqual(gw.RouteParentStatus{}, gw.RouteParentStatus{}))
	require.False(t, parentStatusEqual(gw.RouteParentStatus{}, gw.RouteParentStatus{
		ControllerName: "other",
	}))
	require.False(t, parentStatusEqual(gw.RouteParentStatus{}, gw.RouteParentStatus{
		ParentRef: gw.ParentRef{
			Name: "other",
		},
	}))
}

func TestGatewayStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, gatewayStatusEqual(gw.GatewayStatus{}, gw.GatewayStatus{}))
	require.False(t, gatewayStatusEqual(gw.GatewayStatus{}, gw.GatewayStatus{
		Conditions: []meta.Condition{{}},
	}))
	require.False(t, gatewayStatusEqual(gw.GatewayStatus{}, gw.GatewayStatus{
		Listeners: []gw.ListenerStatus{{}},
	}))
}
