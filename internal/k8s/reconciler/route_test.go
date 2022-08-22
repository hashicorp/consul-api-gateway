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
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service/mocks"
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

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	require.NoError(t, newK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Validate(context.Background()))

	require.True(t, newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).IsValid())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)

	reference := gwv1alpha2.BackendObjectReference{
		Name: "expected",
	}
	resolved := &service.ResolvedReference{
		Type:      service.ConsulServiceReference,
		Reference: &service.BackendReference{},
	}

	route := newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	}, K8sRouteConfig{
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
	})

	resolver.EXPECT().Resolve(gomock.Any(), route.GetNamespace(), reference).Return(resolved, nil)
	require.NoError(t, route.Validate(context.Background()))
	require.True(t, route.IsValid())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), route.GetNamespace(), reference).Return(nil, expected)
	require.Equal(t, expected, route.Validate(context.Background()))

	resolver.EXPECT().Resolve(gomock.Any(), route.GetNamespace(), reference).Return(nil, service.NewK8sResolutionError("error"))
	require.NoError(t, route.Validate(context.Background()))
	require.False(t, route.IsValid())
}

func TestRouteValidateDontAllowCrossNamespace(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	//set up backend ref with a different namespace
	namespace := gwv1alpha2.Namespace("test")
	route := newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: gwv1alpha2.BackendObjectReference{
							Name:      "expected",
							Namespace: &namespace,
						},
					},
				}},
			}},
		},
	}, K8sRouteConfig{
		Client:   client,
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
	})

	client.EXPECT().
		GetReferenceGrantsInNamespace(gomock.Any(), gomock.Any()).
		Return([]gwv1alpha2.ReferenceGrant{
			{
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{},
					To:   []gwv1alpha2.ReferenceGrantTo{},
				},
			},
		}, nil)

	// FUTURE Assert appropriate status set on route and !route.IsValid() once ReferenceGrant requirement is enforced
	_ = route.Validate(context.Background())
}

// TestRouteValidateAllowCrossNamespaceWithReferenceGrant verifies that a cross-namespace
// route + backend combination is allowed if an applicable ReferenceGrant is found.
func TestRouteValidateAllowCrossNamespaceWithReferenceGrant(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	//set up backend ref with a different namespace
	backendGroup := gwv1alpha2.Group("")
	backendKind := gwv1alpha2.Kind("Service")
	backendNamespace := gwv1alpha2.Namespace("namespace2")
	backendName := gwv1alpha2.ObjectName("backend2")
	route := newK8sRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace1"},
		TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: gwv1alpha2.BackendObjectReference{
							Group:     &backendGroup,
							Kind:      &backendKind,
							Name:      backendName,
							Namespace: &backendNamespace,
						},
					},
				}},
			}},
		},
	}, K8sRouteConfig{
		Client:   client,
		Logger:   hclog.NewNullLogger(),
		Resolver: resolver,
	})

	refGrant := gwv1alpha2.ReferenceGrant{
		TypeMeta:   meta.TypeMeta{},
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace2"},
		Spec: gwv1alpha2.ReferenceGrantSpec{
			From: []gwv1alpha2.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gwv1alpha2.ReferenceGrantTo{{
				Group: "",
				Kind:  "Service",
				Name:  &backendName,
			}},
		},
	}

	client.EXPECT().
		GetReferenceGrantsInNamespace(gomock.Any(), gomock.Any()).
		Return([]gwv1alpha2.ReferenceGrant{refGrant}, nil)

	resolver.EXPECT().
		Resolve(gomock.Any(), route.GetNamespace(), gomock.Any()).
		Return(&service.ResolvedReference{Type: service.ConsulServiceReference, Reference: &service.BackendReference{}}, nil)

	require.NoError(t, route.Validate(context.Background()))
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
	})))

	require.NotNil(t, newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
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
