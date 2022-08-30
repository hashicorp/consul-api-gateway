package reconciler

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestRouteID(t *testing.T) {
	t.Parallel()

	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	require.Equal(t, "http-namespace/name", newK8sRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tcp-namespace/name", newK8sRoute(&gwv1alpha2.TCPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "", newK8sRoute(&core.Pod{
		ObjectMeta: meta,
	}, config).ID())
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

	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gwv1alpha2.CommonRouteSpec{
		ParentRefs: []gwv1alpha2.ParentReference{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, newK8sRoute(&gwv1alpha2.TCPRoute{
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, gwv1alpha2.CommonRouteSpec{}, newK8sRoute(&core.Pod{}, config).CommonRouteSpec())
}

func TestRouteSetStatus(t *testing.T) {
	t.Parallel()

	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gwv1alpha2.RouteStatus{
		Parents: []gwv1alpha2.RouteParentStatus{{
			ParentRef: gwv1alpha2.ParentReference{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gwv1alpha2.HTTPRoute{}
	route := newK8sRoute(httpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gwv1alpha2.TCPRoute{}
	route = newK8sRoute(tcpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = newK8sRoute(&core.Pod{}, config)
	route.SetStatus(expected)
	require.Equal(t, gwv1alpha2.RouteStatus{}, route.routeStatus())
}

func TestRouteParents(t *testing.T) {
	t.Parallel()

	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gwv1alpha2.CommonRouteSpec{
		ParentRefs: []gwv1alpha2.ParentReference{{
			Name: "expected",
		}},
	}

	parents := newK8sRoute(&gwv1alpha2.HTTPRoute{Spec: gwv1alpha2.HTTPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = newK8sRoute(&gwv1alpha2.TCPRoute{Spec: gwv1alpha2.TCPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	require.Nil(t, newK8sRoute(&core.Pod{}, config).Parents())
}

func TestRouteMatchesHostname(t *testing.T) {
	t.Parallel()

	hostname := gwv1beta1.Hostname("domain.test")

	require.True(t, newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Hostnames: []gwv1alpha2.Hostname{"*"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))

	require.False(t, newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Hostnames: []gwv1alpha2.Hostname{"other.text"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))

	// check where the underlying route doesn't implement
	// a matching routine
	require.True(t, newK8sRoute(&gwv1alpha2.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))
}

func TestRouteResolve(t *testing.T) {
	t.Parallel()

	gateway := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}
	listener := gwv1beta1.Listener{}

	require.Nil(t, newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(nil))

	require.Nil(t, newK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		State:  &state.ListenerState{},
	})))

	require.NotNil(t, newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		State:  &state.ListenerState{},
	})))
}

func TestRouteSyncStatus(t *testing.T) {
	t.Parallel()

	gateway := newK8sGateway(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	inner := &gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gwv1alpha2.HTTPRouteStatus{
			RouteStatus: gwv1alpha2.RouteStatus{
				Parents: []gwv1alpha2.RouteParentStatus{{
					ParentRef: gwv1alpha2.ParentReference{
						Name: "expected",
					},
					ControllerName: "expected",
				}, {
					ParentRef: gwv1alpha2.ParentReference{
						Name: "expected",
					},
					ControllerName: "other",
				}, {
					ParentRef: gwv1alpha2.ParentReference{
						Name: "other",
					},
					ControllerName: "other",
				}},
			},
		},
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := clientMocks.NewMockClient(ctrl)

	logger := hclog.New(&hclog.LoggerOptions{
		Output: io.Discard,
	})
	logger.SetLevel(hclog.Trace)
	route := newK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         logger,
		Client:         client,
	})
	route.OnBound(gateway)

	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.True(t, errors.Is(route.SyncStatus(context.Background()), expected))

	require.NoError(t, route.SyncStatus(context.Background()))
}
