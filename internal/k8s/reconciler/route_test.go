// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestRouteID(t *testing.T) {
	t.Parallel()

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	assert.Equal(t, "http-namespace/name", newK8sRoute(&gwv1alpha2.HTTPRoute{ObjectMeta: meta}, state.NewRouteState()).ID())
	assert.Equal(t, "tcp-namespace/name", newK8sRoute(&gwv1alpha2.TCPRoute{ObjectMeta: meta}, state.NewRouteState()).ID())
	assert.Equal(t, "", newK8sRoute(&core.Pod{ObjectMeta: meta}, state.NewRouteState()).ID())
}

func TestHTTPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http-namespace/name", HTTPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}

func TestTCPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "tcp-namespace/name", TCPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}

func TestRouteCommonRouteSpec(t *testing.T) {
	t.Parallel()

	expected := gwv1alpha2.CommonRouteSpec{
		ParentRefs: []gwv1alpha2.ParentReference{{
			Name: "expected",
		}},
	}

	assert.Equal(t, expected, newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).commonRouteSpec())
	assert.Equal(t, expected, newK8sRoute(&gwv1alpha2.TCPRoute{
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).commonRouteSpec())
	assert.Equal(t, gwv1alpha2.CommonRouteSpec{}, newK8sRoute(&core.Pod{}, state.NewRouteState()).commonRouteSpec())
}

func TestRouteSetStatus(t *testing.T) {
	t.Parallel()

	expected := gwv1alpha2.RouteStatus{
		Parents: []gwv1alpha2.RouteParentStatus{{
			ParentRef: gwv1alpha2.ParentReference{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gwv1alpha2.HTTPRoute{}
	route := newK8sRoute(httpRoute, state.NewRouteState())
	route.setStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gwv1alpha2.TCPRoute{}
	route = newK8sRoute(tcpRoute, state.NewRouteState())
	route.setStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = newK8sRoute(&core.Pod{}, state.NewRouteState())
	route.setStatus(expected)
	require.Equal(t, gwv1alpha2.RouteStatus{}, route.routeStatus())
}

func TestRouteMatchesHostname(t *testing.T) {
	t.Parallel()

	hostname := gwv1beta1.Hostname("domain.test")

	assert.True(t, newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Hostnames: []gwv1alpha2.Hostname{"*"},
		},
	}, state.NewRouteState()).matchesHostname(&hostname))

	assert.False(t, newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Hostnames: []gwv1alpha2.Hostname{"other.text"},
		},
	}, state.NewRouteState()).matchesHostname(&hostname))

	// check where the underlying route doesn't implement
	// a matching routine
	assert.True(t, newK8sRoute(&gwv1alpha2.TCPRoute{}, state.NewRouteState()).matchesHostname(&hostname))
}

func TestRouteResolve(t *testing.T) {
	t.Parallel()

	gateway := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}
	listener := gwv1beta1.Listener{}

	assert.Nil(t, newK8sRoute(&core.Pod{}, state.NewRouteState()).resolve("", gateway, listener))

	assert.NotNil(t, newK8sRoute(&gwv1alpha2.HTTPRoute{}, state.NewRouteState()).resolve("", gateway, listener))
}
