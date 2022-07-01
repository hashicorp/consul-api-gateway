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
	"github.com/hashicorp/consul-api-gateway/internal/store"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
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

	require.Equal(t, "http-namespace/name", NewK8sRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "udp-namespace/name", NewK8sRoute(&gwv1alpha2.UDPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tcp-namespace/name", NewK8sRoute(&gwv1alpha2.TCPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tls-namespace/name", NewK8sRoute(&gwv1alpha2.TLSRoute{
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

	expected := gwv1alpha2.CommonRouteSpec{
		ParentRefs: []gwv1alpha2.ParentReference{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, NewK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gwv1alpha2.UDPRoute{
		Spec: gwv1alpha2.UDPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gwv1alpha2.TCPRoute{
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gwv1alpha2.TLSRoute{
		Spec: gwv1alpha2.TLSRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, gwv1alpha2.CommonRouteSpec{}, NewK8sRoute(&core.Pod{}, config).CommonRouteSpec())
}

func TestRouteFilterParentStatuses(t *testing.T) {
	t.Parallel()

	route := NewK8sRoute(&gwv1alpha2.HTTPRoute{
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
	}, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})

	route.OnBound(NewK8sGateway(&gwv1beta1.Gateway{
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

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	inner := &gwv1alpha2.TLSRoute{
		Spec: gwv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gwv1alpha2.TLSRouteStatus{
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

	// check route ref
	route = NewK8sRoute(inner, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})
	route.resolutionErrors.Add(service.NewRefNotPermittedError("not found"))
	route.OnBindFailed(nil, gateway)
	statuses = route.MergedStatus().Parents
	require.Len(t, statuses, 3)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "expected", string(statuses[0].ControllerName))
	require.Equal(t, "not found", statuses[0].Conditions[1].Message)
	require.Equal(t, RouteConditionReasonRefNotPermitted, statuses[0].Conditions[1].Reason)

	// check binding for non-existent route
	gateway = NewK8sGateway(&gwv1beta1.Gateway{
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

	route := NewK8sRoute(&gwv1alpha2.TCPRoute{
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gwv1alpha2.TCPRouteStatus{
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
	}, K8sRouteConfig{
		ControllerName: "expected",
		Logger:         hclog.NewNullLogger(),
	})
	route.SetStatus(route.MergedStatus())

	require.False(t, route.NeedsStatusUpdate())

	route.OnBound(NewK8sGateway(&gwv1beta1.Gateway{
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

	expected := gwv1alpha2.RouteStatus{
		Parents: []gwv1alpha2.RouteParentStatus{{
			ParentRef: gwv1alpha2.ParentReference{
				Name: "expected",
			},
		}},
	}

	httpRoute := &gwv1alpha2.HTTPRoute{}
	route := NewK8sRoute(httpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, httpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tcpRoute := &gwv1alpha2.TCPRoute{}
	route = NewK8sRoute(tcpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, tcpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	tlsRoute := &gwv1alpha2.TLSRoute{}
	route = NewK8sRoute(tlsRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, tlsRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	udpRoute := &gwv1alpha2.UDPRoute{}
	route = NewK8sRoute(udpRoute, config)
	route.SetStatus(expected)
	require.Equal(t, expected, udpRoute.Status.RouteStatus)
	require.Equal(t, expected, route.routeStatus())

	route = NewK8sRoute(&core.Pod{}, config)
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

	parents := NewK8sRoute(&gwv1alpha2.HTTPRoute{Spec: gwv1alpha2.HTTPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewK8sRoute(&gwv1alpha2.TCPRoute{Spec: gwv1alpha2.TCPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewK8sRoute(&gwv1alpha2.TLSRoute{Spec: gwv1alpha2.TLSRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	parents = NewK8sRoute(&gwv1alpha2.UDPRoute{Spec: gwv1alpha2.UDPRouteSpec{CommonRouteSpec: expected}}, config).Parents()
	require.Equal(t, expected.ParentRefs, parents)

	require.Nil(t, NewK8sRoute(&core.Pod{}, config).Parents())
}

func TestRouteMatchesHostname(t *testing.T) {
	t.Parallel()

	hostname := gwv1beta1.Hostname("domain.test")

	require.True(t, NewK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Hostnames: []gwv1alpha2.Hostname{"*"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))

	require.False(t, NewK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Hostnames: []gwv1alpha2.Hostname{"other.text"},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))

	// check where the underlying route doesn't implement
	// a matching routine
	require.True(t, NewK8sRoute(&gwv1alpha2.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).MatchesHostname(&hostname))
}

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	require.NoError(t, NewK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Validate(context.Background()))

	require.True(t, NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
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

	resolver.EXPECT().Resolve(gomock.Any(), reference).Return(resolved, nil)

	route := NewK8sRoute(&gwv1alpha2.HTTPRoute{
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
	require.NoError(t, route.Validate(context.Background()))
	require.True(t, route.IsValid())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), reference).Return(nil, expected)
	require.Equal(t, expected, route.Validate(context.Background()))

	resolver.EXPECT().Resolve(gomock.Any(), reference).Return(nil, service.NewK8sResolutionError("error"))
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
	route := NewK8sRoute(&gwv1alpha2.HTTPRoute{
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
	route := NewK8sRoute(&gwv1alpha2.HTTPRoute{
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
		Resolve(gomock.Any(), gomock.Any()).
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

	require.Nil(t, NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(nil))

	require.Nil(t, NewK8sRoute(&core.Pod{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}).Resolve(NewK8sListener(gateway, listener, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})))

	require.NotNil(t, NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
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
	route = NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other.resolutionErrors.Add(service.NewConsulResolutionError("error"))
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))
	route = NewK8sRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			ResourceVersion: "1",
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNewer, route.Compare(other))

	route = NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gwv1alpha2.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// tcp route comparison
	route = NewK8sRoute(&gwv1alpha2.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gwv1alpha2.TCPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other = NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// tls route comparison
	route = NewK8sRoute(&gwv1alpha2.TLSRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gwv1alpha2.TLSRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other = NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// udp route comparison
	route = NewK8sRoute(&gwv1alpha2.UDPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sRoute(&gwv1alpha2.UDPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, route.Compare(other))
	other = NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNotEqual, route.Compare(other))

	// mismatched types
	require.Equal(t, store.CompareResultInvalid, route.Compare(storeMocks.NewMockRoute(nil)))
}

func TestRouteSyncStatus(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name: "expected",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	inner := &gwv1alpha2.TLSRoute{
		Spec: gwv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "expected",
				}, {
					Name: "other",
				}},
			},
		},
		Status: gwv1alpha2.TLSRouteStatus{
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
