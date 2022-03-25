package reconciler

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteID(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	require.Equal(t, "http-namespace/name", factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta,
	}, state.NewRouteState()).ID())
	require.Equal(t, "tcp-namespace/name", factory.NewRoute(&gw.TCPRoute{
		ObjectMeta: meta,
	}, state.NewRouteState()).ID())
	require.Equal(t, "", factory.NewRoute(&core.Pod{
		ObjectMeta: meta,
	}, state.NewRouteState()).ID())
}

func TestRouteCommonRouteSpec(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).CommonRouteSpec())
	require.Equal(t, expected, factory.NewRoute(&gw.UDPRoute{
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).CommonRouteSpec())
	require.Equal(t, expected, factory.NewRoute(&gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).CommonRouteSpec())
	require.Equal(t, expected, factory.NewRoute(&gw.TLSRoute{
		Spec: gw.TLSRouteSpec{
			CommonRouteSpec: expected,
		},
	}, state.NewRouteState()).CommonRouteSpec())
	require.Equal(t, gw.CommonRouteSpec{}, factory.NewRoute(&core.Pod{}, state.NewRouteState()).CommonRouteSpec())
}

func TestRouteSetStatus(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	expected := gw.RouteStatus{
		Parents: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gw.HTTPRoute{}
	route := factory.NewRoute(httpRoute, state.NewRouteState())
	route.SetStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gw.TCPRoute{}
	route = factory.NewRoute(tcpRoute, state.NewRouteState())
	route.SetStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tlsRoute := &gw.TLSRoute{}
	route = factory.NewRoute(tlsRoute, state.NewRouteState())
	route.SetStatus(expected)
	require.Equal(t, expected, tlsRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	udpRoute := &gw.UDPRoute{}
	route = factory.NewRoute(udpRoute, state.NewRouteState())
	route.SetStatus(expected)
	require.Equal(t, expected, udpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = factory.NewRoute(&core.Pod{}, state.NewRouteState())
	route.SetStatus(expected)
	require.Equal(t, gw.RouteStatus{}, route.routeStatus())
}

func TestRouteParents(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	parents := factory.NewRoute(&gw.HTTPRoute{Spec: gw.HTTPRouteSpec{CommonRouteSpec: expected}}, state.NewRouteState()).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = factory.NewRoute(&gw.TCPRoute{Spec: gw.TCPRouteSpec{CommonRouteSpec: expected}}, state.NewRouteState()).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	require.Nil(t, factory.NewRoute(&core.Pod{}, state.NewRouteState()).Parents())
}

func TestRouteMatchesHostname(t *testing.T) {
	t.Parallel()

	hostname := gw.Hostname("domain.test")

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	require.True(t, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"*"},
		},
	}, state.NewRouteState()).MatchesHostname(&hostname))

	require.False(t, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"other.text"},
		},
	}, state.NewRouteState()).MatchesHostname(&hostname))

	// check where the underlying route doesn't implement
	// a matching routine
	require.True(t, factory.NewRoute(&gw.TCPRoute{}, state.NewRouteState()).MatchesHostname(&hostname))
}

func TestRouteSyncStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := clientMocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger:         hclog.NewNullLogger(),
		Client:         client,
		ControllerName: "expected",
	})

	inner := &gw.TLSRoute{
		Spec: gw.TLSRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gw.TLSRouteStatus{
			RouteStatus: gw.RouteStatus{
				Parents: []gw.RouteParentStatus{{
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					ControllerName: "expected",
				}, {
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					ControllerName: "other",
				}, {
					ParentRef: gw.ParentRef{
						Name: "other",
					},
					ControllerName: "other",
				}},
			},
		},
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Output: io.Discard,
	})
	logger.SetLevel(hclog.Trace)
	route := factory.NewRoute(inner, state.NewRouteState())
	route.RouteState.ParentStatuses.Bound(common.AsJSON(gw.ParentRef{
		Name: "expected",
	}))

	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.True(t, errors.Is(route.SyncStatus(context.Background()), expected))

	require.NoError(t, route.SyncStatus(context.Background()))
}

func TestHTTPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http-namespace/name", HTTPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}

func TestTCPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "tcp-namespace/name", TCPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}
