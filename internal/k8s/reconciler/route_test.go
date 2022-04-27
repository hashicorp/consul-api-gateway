package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service/mocks"
)

func TestRouteID(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	config := state.NewRouteState()

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	require.Equal(t, "http-namespace/name", factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tcp-namespace/name", factory.NewRoute(&gw.TCPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "", factory.NewRoute(&core.Pod{
		ObjectMeta: meta,
	}, config).ID())
}

func TestRouteCommonRouteSpec(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	config := state.NewRouteState()

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, factory.NewRoute(&gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, gw.CommonRouteSpec{}, factory.NewRoute(&core.Pod{}, config).CommonRouteSpec())
}

func TestRouteSetStatus(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	config := state.NewRouteState()

	expected := gw.RouteStatus{
		Parents: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gw.HTTPRoute{}
	route := factory.NewRoute(httpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gw.TCPRoute{}
	route = factory.NewRoute(tcpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = factory.NewRoute(&core.Pod{}, config)
	route.SetStatus(expected)
	require.Equal(t, gw.RouteStatus{}, route.routeStatus())
}

func TestRouteParents(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	config := state.NewRouteState()

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	parents := factory.NewRoute(&gw.HTTPRoute{Spec: gw.HTTPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = factory.NewRoute(&gw.TCPRoute{Spec: gw.TCPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	require.Nil(t, factory.NewRoute(&core.Pod{}, config).Parents())
}

func TestRouteMatchesHostname(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	hostname := gw.Hostname("domain.test")

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

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
	})

	require.NoError(t, factory.NewRoute(&core.Pod{}, state.NewRouteState()).Validate(context.Background()))

	require.True(t, factory.NewRoute(&gw.HTTPRoute{}, state.NewRouteState()).IsValid())

	reference := gw.BackendObjectReference{
		Name: "expected",
	}
	resolved := &service.ResolvedReference{
		Type:      service.ConsulServiceReference,
		Reference: &service.BackendReference{},
	}

	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(resolved, nil)

	route := factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	}, state.NewRouteState())
	require.NoError(t, route.Validate(context.Background()))
	require.True(t, route.IsValid())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, expected)
	require.Equal(t, expected, route.Validate(context.Background()))

	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, service.NewK8sResolutionError("error"))
	require.NoError(t, route.Validate(context.Background()))
	require.False(t, route.IsValid())
}

func TestRouteValidateDontAllowCrossNamespace(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
		Client:   client,
	})

	//set up backend ref with a different namespace
	namespace := gw.Namespace("test")
	route := factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: gw.BackendObjectReference{
							Name:      "expected",
							Namespace: &namespace,
						},
					},
				}},
			}},
		},
	}, state.NewRouteState())

	client.EXPECT().
		GetReferencePoliciesInNamespace(gomock.Any(), gomock.Any()).
		Return([]gw.ReferencePolicy{
			{
				Spec: gw.ReferencePolicySpec{
					From: []gw.ReferencePolicyFrom{},
					To:   []gw.ReferencePolicyTo{},
				},
			},
		}, nil)

	// FUTURE Assert appropriate status set on route and !route.IsValid() once ReferencePolicy requirement is enforced
	_ = route.Validate(context.Background())
}

// TestRouteValidateAllowCrossNamespaceWithReferencePolicy verifies that a cross-namespace
// route + backend combination is allowed if an applicable ReferencePolicy is found.
func TestRouteValidateAllowCrossNamespaceWithReferencePolicy(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
		Client:   client,
	})

	//set up backend ref with a different namespace
	backendGroup := gw.Group("")
	backendKind := gw.Kind("Service")
	backendNamespace := gw.Namespace("namespace2")
	backendName := gw.ObjectName("backend2")
	route := factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace1"},
		TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: gw.BackendObjectReference{
							Group:     &backendGroup,
							Kind:      &backendKind,
							Name:      backendName,
							Namespace: &backendNamespace,
						},
					},
				}},
			}},
		},
	}, state.NewRouteState())

	referencePolicy := gw.ReferencePolicy{
		TypeMeta:   meta.TypeMeta{},
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace2"},
		Spec: gw.ReferencePolicySpec{
			From: []gw.ReferencePolicyFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gw.ReferencePolicyTo{{
				Group: "",
				Kind:  "Service",
				Name:  &backendName,
			}},
		},
	}

	client.EXPECT().
		GetReferencePoliciesInNamespace(gomock.Any(), gomock.Any()).
		Return([]gw.ReferencePolicy{referencePolicy}, nil)

	resolver.EXPECT().
		Resolve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&service.ResolvedReference{Type: service.ConsulServiceReference, Reference: &service.BackendReference{}}, nil)

	require.NoError(t, route.Validate(context.Background()))
}

func TestRouteResolve(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gateway := &gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}
	listener := gw.Listener{}

	require.Nil(t, factory.NewRoute(&gw.HTTPRoute{}, state.NewRouteState()).Resolve(nil))

	require.Nil(t, factory.NewRoute(&core.Pod{}, state.NewRouteState()).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})))

	require.NotNil(t, factory.NewRoute(&gw.HTTPRoute{}, state.NewRouteState()).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})))
}

func TestRouteSyncStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := clientMocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})

	g := &gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: g,
		State:   state.InitialGatewayState(g),
	})
	inner := &gw.TCPRoute{
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
	}
	route := factory.NewRoute(inner, state.NewRouteState())
	route.OnBound(gateway)

	expected := errors.New("expected")
	client.EXPECT().UpdateStatus(gomock.Any(), inner).Return(expected)
	require.True(t, errors.Is(route.SyncStatus(context.Background()), expected))

	require.NoError(t, route.SyncStatus(context.Background()))
}
