package reconciler

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteID(t *testing.T) {
	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	require.Equal(t, "http-namespace/name", NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "udp-namespace/name", NewK8sRoute(&gw.UDPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tcp-namespace/name", NewK8sRoute(&gw.TCPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tls-namespace/name", NewK8sRoute(&gw.TLSRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "", NewK8sRoute(&core.Pod{
		ObjectMeta: meta,
	}, config).ID())
}

func TestRouteCommonRouteSpec(t *testing.T) {
	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gw.UDPRoute{
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gw.TLSRoute{
		Spec: gw.TLSRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, gw.CommonRouteSpec{}, NewK8sRoute(&core.Pod{}, config).CommonRouteSpec())
}

func TestRouteFilterParentStatuses(t *testing.T) {
	route := NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gw.HTTPRouteStatus{
			RouteStatus: gw.RouteStatus{
				Parents: []gw.RouteParentStatus{{
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					Controller: "expected",
				}, {
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					Controller: "other",
				}, {
					ParentRef: gw.ParentRef{
						Name: "other",
					},
					Controller: "other",
				}},
			},
		},
	}, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})

	route.OnBound(NewK8sGateway(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	}))

	statuses := route.FilterParentStatuses()
	require.Len(t, statuses, 2)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "other", string(statuses[0].Controller))
	require.Equal(t, "other", statuses[1].ParentRef.Name)
	require.Equal(t, "other", string(statuses[1].Controller))
}

func TestRouteMergedStatusAndBinding(t *testing.T) {
	gateway := NewK8sGateway(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
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
					Controller: "expected",
				}, {
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					Controller: "other",
				}, {
					ParentRef: gw.ParentRef{
						Name: "other",
					},
					Controller: "other",
				}},
			},
		},
	}
	route := NewK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})

	route.OnBound(gateway)

	statuses := route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Len(t, statuses[0].Conditions, 2)
	require.Equal(t, "Route accepted.", statuses[0].Conditions[0].Message)
	require.Equal(t, "expected", statuses[1].ParentRef.Name)
	require.Equal(t, "other", string(statuses[1].Controller))
	require.Equal(t, "other", statuses[2].ParentRef.Name)
	require.Equal(t, "other", string(statuses[2].Controller))

	route.OnBindFailed(errors.New("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonBindError, statuses[0].Conditions[0].Reason)

	route.OnBindFailed(NewBindErrorHostnameMismatch("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonListenerHostnameMismatch, statuses[0].Conditions[0].Reason)

	route.OnBindFailed(NewBindErrorListenerNamespacePolicy("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonListenerNamespacePolicy, statuses[0].Conditions[0].Reason)

	route.OnBindFailed(NewBindErrorRouteKind("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonInvalidRouteKind, statuses[0].Conditions[0].Reason)

	route.OnBound(gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Equal(t, "Route accepted.", statuses[0].Conditions[0].Message)

	route.OnGatewayRemoved(gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 2)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "other", string(statuses[0].Controller))
	require.Equal(t, "other", statuses[1].ParentRef.Name)
	require.Equal(t, "other", string(statuses[1].Controller))

	// check creating a status on bind failure when it's not there
	route = NewK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})

	route.OnBindFailed(errors.New("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", statuses[0].ParentRef.Name)
	require.Equal(t, "expected", string(statuses[0].Controller))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonBindError, statuses[0].Conditions[0].Reason)

	// check binding for non-existent route
	gateway = NewK8sGateway(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "nothing",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	route.OnBound(gateway)
	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	route.OnBindFailed(errors.New("expected"), gateway)
	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
}

func TestRouteNeedsStatusUpdate(t *testing.T) {
	route := NewK8sRoute(&gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gw.TCPRouteStatus{
			RouteStatus: gw.RouteStatus{
				Parents: []gw.RouteParentStatus{{
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					Controller: "expected",
				}, {
					ParentRef: gw.ParentRef{
						Name: "expected",
					},
					Controller: "other",
				}, {
					ParentRef: gw.ParentRef{
						Name: "other",
					},
					Controller: "other",
				}},
			},
		},
	}, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})
	route.SetStatus(route.MergedStatus())

	require.False(t, route.NeedsStatusUpdate())

	route.OnBound(NewK8sGateway(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	}))

	require.True(t, route.NeedsStatusUpdate())

	route.SetStatus(route.MergedStatus())

	require.False(t, route.NeedsStatusUpdate())
}

func TestRouteSetStatus(t *testing.T) {
	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gw.RouteStatus{
		Parents: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gw.HTTPRoute{}
	route := NewK8sRoute(httpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gw.TCPRoute{}
	route = NewK8sRoute(tcpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tlsRoute := &gw.TLSRoute{}
	route = NewK8sRoute(tlsRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, tlsRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	udpRoute := &gw.UDPRoute{}
	route = NewK8sRoute(udpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, udpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = NewK8sRoute(&core.Pod{}, config)
	route.SetStatus(expected)
	require.Equal(t, gw.RouteStatus{}, route.routeStatus())
}

func TestRouteParents(t *testing.T) {
	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	parents := NewK8sRoute(&gw.HTTPRoute{Spec: gw.HTTPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewK8sRoute(&gw.TCPRoute{Spec: gw.TCPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewK8sRoute(&gw.TLSRoute{Spec: gw.TLSRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewK8sRoute(&gw.UDPRoute{Spec: gw.UDPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	require.Nil(t, NewK8sRoute(&core.Pod{}, config).Parents())
}

func TestRouteMatchesHostname(t *testing.T) {
	hostname := gw.Hostname("domain.test")

	require.True(t, NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"*"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))

	require.False(t, NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Hostnames: []gw.Hostname{"other.text"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))

	// check where the underlying route doesn't implement
	// a matching routine
	require.True(t, NewK8sRoute(&gw.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))
}

func TestRouteIsValid(t *testing.T) {
	require.True(t, NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).IsValid())
}
