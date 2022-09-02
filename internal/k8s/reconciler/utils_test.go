package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

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
