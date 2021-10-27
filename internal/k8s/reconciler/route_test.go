package reconciler

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
	t.Parallel()

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
	t.Parallel()

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
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "other", string(statuses[0].ControllerName))
	require.Equal(t, "other", string(statuses[1].ParentRef.Name))
	require.Equal(t, "other", string(statuses[1].ControllerName))
}

func TestRouteMergedStatusAndBinding(t *testing.T) {
	t.Parallel()

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
	route := NewK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})

	route.OnBound(gateway)

	statuses := route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Len(t, statuses[0].Conditions, 2)
	require.Equal(t, "Route accepted.", statuses[0].Conditions[0].Message)
	require.Equal(t, "expected", string(statuses[1].ParentRef.Name))
	require.Equal(t, "other", string(statuses[1].ControllerName))
	require.Equal(t, "other", string(statuses[2].ParentRef.Name))
	require.Equal(t, "other", string(statuses[2].ControllerName))

	route.OnBindFailed(errors.New("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonBindError, statuses[0].Conditions[0].Reason)

	route.OnBindFailed(NewBindErrorHostnameMismatch("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonListenerHostnameMismatch, statuses[0].Conditions[0].Reason)

	route.OnBindFailed(NewBindErrorListenerNamespacePolicy("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonListenerNamespacePolicy, statuses[0].Conditions[0].Reason)

	route.OnBindFailed(NewBindErrorRouteKind("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Equal(t, "expected", statuses[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonInvalidRouteKind, statuses[0].Conditions[0].Reason)

	route.OnBound(gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Equal(t, "Route accepted.", statuses[0].Conditions[0].Message)

	route.OnGatewayRemoved(gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 2)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "other", string(statuses[0].ControllerName))
	require.Equal(t, "other", string(statuses[1].ParentRef.Name))
	require.Equal(t, "other", string(statuses[1].ControllerName))

	// check creating a status on bind failure when it's not there
	route = NewK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})

	route.OnBindFailed(errors.New("expected"), gateway)

	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	require.NoError(t, NewK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Validate(context.Background()))

	require.True(t, NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).IsValid())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)

	reference := gw.BackendObjectReference{
		Name: "expected",
	}
	resolved := &service.ResolvedReference{
		Type:      service.ConsulServiceReference,
		Reference: &service.BackendReference{},
	}

	resolver.EXPECT().Resolve(gomock.Any(), reference).Return(resolved, nil)

	route := NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	}, K8sRouteConfig{
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
	})
	require.NoError(t, route.Validate(context.Background()))
	require.True(t, route.IsValid())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), reference).Return(nil, expected)
	require.Equal(t, expected, route.Validate(context.Background()))

	resolver.EXPECT().Resolve(gomock.Any(), reference).Return(nil, service.NewK8sResolutionError("error"))
	require.NoError(t, route.Validate(context.Background()))
	require.False(t, route.IsValid())
}

func TestRouteResolve(t *testing.T) {
	t.Parallel()

	gateway := &gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}
	listener := gw.Listener{}

	require.Nil(t, NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(nil))

	require.Nil(t, NewK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})))

	require.NotNil(t, NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})))
}

func TestRouteCompare(t *testing.T) {
	t.Parallel()

	// invalid route comparison
	route := NewK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other := NewK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})

	require.Equal(t, store.CompareResultNotEqual, route.Compare(route))
	require.Equal(t, store.CompareResultInvalid, route.Compare(nil))
	route = nil
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// http route comparison
	route = NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other.resolutionErrors.Add(service.NewConsulResolutionError("error"))
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))
	route = NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			ResourceVersion: "1",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNewer, route.Compare(other))

	route = NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gw.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// tcp route comparison
	route = NewK8sRoute(&gw.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gw.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other = NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// tls route comparison
	route = NewK8sRoute(&gw.TLSRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gw.TLSRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other = NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// udp route comparison
	route = NewK8sRoute(&gw.UDPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gw.UDPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other = NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// mismatched types
	require.Equal(t, store.CompareResultInvalid, route.Compare(storeMocks.NewMockRoute(nil)))
}

func TestRouteSyncStatus(t *testing.T) {
	t.Parallel()

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := clientMocks.NewMockClient(ctrl)

	logger := hclog.New(&hclog.LoggerOptions{
		Output: io.Discard,
	})
	logger.SetLevel(hclog.Trace)
	route := NewK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         logger,
		Client:         client,
	})
	route.OnBound(gateway)

	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.True(t, errors.Is(route.SyncStatus(context.Background()), expected))

	client.EXPECT().UpdateStatus(gomock.Any(), inner)
	require.NoError(t, route.SyncStatus(context.Background()))

	// sync again, no status update called
	require.NoError(t, route.SyncStatus(context.Background()))
}
