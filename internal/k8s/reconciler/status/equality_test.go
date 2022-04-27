package status

import (
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

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

	require.True(t, ConditionsEqual([]meta.Condition{}, []meta.Condition{}))
	require.False(t, ConditionsEqual([]meta.Condition{}, []meta.Condition{{}}))
	require.True(t, ConditionsEqual([]meta.Condition{{
		Type: "expected",
	}}, []meta.Condition{{
		Type: "expected",
	}}))
	require.False(t, ConditionsEqual([]meta.Condition{{
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

	require.True(t, ListenerStatusesEqual([]gw.ListenerStatus{}, []gw.ListenerStatus{}))
	require.False(t, ListenerStatusesEqual([]gw.ListenerStatus{}, []gw.ListenerStatus{{}}))
	require.False(t, ListenerStatusesEqual([]gw.ListenerStatus{{
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

	require.True(t, GatewayStatusEqual(gw.GatewayStatus{}, gw.GatewayStatus{}))
	require.False(t, GatewayStatusEqual(gw.GatewayStatus{}, gw.GatewayStatus{
		Conditions: []meta.Condition{{}},
	}))
	require.False(t, GatewayStatusEqual(gw.GatewayStatus{}, gw.GatewayStatus{
		Listeners: []gw.ListenerStatus{{}},
	}))
}
