package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestRouteID(t *testing.T) {
	t.Parallel()

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	require.Equal(t, "http-namespace/name", NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta,
	}, state.NewRouteState()).ID())
	require.Equal(t, "tcp-namespace/name", NewRoute(&gw.TCPRoute{
		ObjectMeta: meta,
	}, state.NewRouteState()).ID())
	require.Equal(t, "", NewRoute(&core.Pod{
		ObjectMeta: meta,
	}, state.NewRouteState()).ID())
}

func TestRouteCommonRouteSpec(t *testing.T) {
	t.Parallel()

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).commonRouteSpec())
	require.Equal(t, expected, NewRoute(&gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).commonRouteSpec())
	require.Equal(t, gw.CommonRouteSpec{}, NewRoute(&core.Pod{}, state.NewRouteState()).commonRouteSpec())
}

func TestRoutesetStatus(t *testing.T) {
	t.Parallel()

	state := state.NewRouteState()

	expected := gw.RouteStatus{
		Parents: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gw.HTTPRoute{}
	route := NewRoute(httpRoute, state)
	route.setStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gw.TCPRoute{}
	route = NewRoute(tcpRoute, state)
	route.setStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = NewRoute(&core.Pod{}, state)
	route.setStatus(expected)
	require.Equal(t, gw.RouteStatus{}, route.routeStatus())
}

func TestRouteParents(t *testing.T) {
	t.Parallel()

	state := state.NewRouteState()

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	parents := NewRoute(&gw.HTTPRoute{Spec: gw.HTTPRouteSpec{CommonRouteSpec: expected}}, state).parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewRoute(&gw.TCPRoute{Spec: gw.TCPRouteSpec{CommonRouteSpec: expected}}, state).parents()
	require.Equal(t, expected.ParentRefs, parents)

	require.Nil(t, NewRoute(&core.Pod{}, state).parents())
}

func TestRouteMatchesHostname(t *testing.T) {
	t.Parallel()

	hostname := gw.Hostname("domain.test")

	require.True(t, NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"*"},
		},
	}, state.NewRouteState()).matchesHostname(&hostname))

	require.False(t, NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"other.text"},
		},
	}, state.NewRouteState()).matchesHostname(&hostname))

	// check where the underlying route doesn't implement
	// a matching routine
	require.True(t, NewRoute(&gw.TCPRoute{}, state.NewRouteState()).matchesHostname(&hostname))
}

func TestRouteResolve(t *testing.T) {
	t.Parallel()

	gateway := &gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}
	listener := gw.Listener{}

	require.Nil(t, NewRoute(&core.Pod{}, state.NewRouteState()).resolve("", gateway, listener))

	require.NotNil(t, NewRoute(&gw.HTTPRoute{}, state.NewRouteState()).resolve("", gateway, listener))
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
