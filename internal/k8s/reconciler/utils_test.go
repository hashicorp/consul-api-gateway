package reconciler

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
)

func TestRouteMatchesListener(t *testing.T) {
	t.Parallel()

	listenerName := gwv1beta1.SectionName("name")
	routeSectionName := gwv1alpha2.SectionName("name")
	can, must := routeMatchesListener(listenerName, &routeSectionName)
	require.True(t, can)
	require.True(t, must)

	can, must = routeMatchesListener(listenerName, nil)
	require.True(t, can)
	require.False(t, must)

	can, must = routeMatchesListener(gwv1beta1.SectionName("other"), &routeSectionName)
	require.False(t, can)
	require.True(t, must)
}

func TestRouteMatchesListenerHostname(t *testing.T) {
	t.Parallel()

	hostname := gwv1beta1.Hostname("name")
	require.True(t, routeMatchesListenerHostname(nil, nil))
	require.True(t, routeMatchesListenerHostname(&hostname, nil))
	require.True(t, routeMatchesListenerHostname(&hostname, []gwv1alpha2.Hostname{"*"}))
	require.False(t, routeMatchesListenerHostname(&hostname, []gwv1alpha2.Hostname{"other"}))
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

func TestRouteKindIsAllowedForListener(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1alpha2.GroupVersion.Group,
		Version: gwv1alpha2.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})

	require.True(t, routeKindIsAllowedForListener(
		[]gwv1beta1.RouteGroupKind{{
			Group: (*gwv1beta1.Group)(&gwv1alpha2.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		factory.NewRoute(&gwv1alpha2.HTTPRoute{TypeMeta: routeMeta})))

	require.False(t, routeKindIsAllowedForListener(
		[]gwv1beta1.RouteGroupKind{{
			Group: (*gwv1beta1.Group)(&gwv1alpha2.GroupVersion.Group),
			Kind:  "TCPRoute",
		}},
		factory.NewRoute(&gwv1alpha2.HTTPRoute{TypeMeta: routeMeta})))
}

func TestRouteAllowedForListenerNamespaces(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	// same
	same := gwv1beta1.NamespacesFromSame

	allowed, err := routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)

	// all
	all := gwv1beta1.NamespacesFromAll
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &all,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	// selector
	selector := gwv1beta1.NamespacesFromSelector

	matchingNamespace := &core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Labels: map[string]string{
				"label":                       "test",
				"kubernetes.io/metadata.name": "expected",
			}}}
	invalidNamespace := &core.Namespace{ObjectMeta: meta.ObjectMeta{Labels: map[string]string{}}}

	client.EXPECT().GetNamespace(context.Background(), types.NamespacedName{Name: "expected"}).Return(invalidNamespace, nil).Times(1)
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)

	client.EXPECT().GetNamespace(context.Background(), types.NamespacedName{Name: "expected"}).Return(matchingNamespace, nil).Times(1)
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	_, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchExpressions: []meta.LabelSelectorRequirement{{
					Key:      "test",
					Operator: meta.LabelSelectorOperator("invalid"),
				}},
			},
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.Error(t, err)

	// unknown
	unknown := gwv1beta1.FromNamespaces("unknown")
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &unknown,
		},
	}, factory.NewRoute(&gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)
}

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

	require.True(t, conditionsEqual([]meta.Condition{}, []meta.Condition{}))
	require.False(t, conditionsEqual([]meta.Condition{}, []meta.Condition{{}}))
	require.True(t, conditionsEqual([]meta.Condition{{
		Type: "expected",
	}}, []meta.Condition{{
		Type: "expected",
	}}))
	require.False(t, conditionsEqual([]meta.Condition{{
		Type: "expected",
	}}, []meta.Condition{{
		Type: "other",
	}}))
}

func TestListenerStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, listenerStatusEqual(gwv1beta1.ListenerStatus{}, gwv1beta1.ListenerStatus{}))
	require.False(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		Name: "expected",
	}, gwv1beta1.ListenerStatus{
		Name: "other",
	}))
	require.False(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		AttachedRoutes: 1,
	}, gwv1beta1.ListenerStatus{
		AttachedRoutes: 2,
	}))

	groupOne := gwv1beta1.Group("group")
	kindOne := gwv1beta1.Kind("kind")
	groupTwo := gwv1beta1.Group("group")
	kindTwo := gwv1beta1.Kind("kind")
	require.True(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))

	groupTwo = gwv1beta1.Group("other")
	require.False(t, listenerStatusEqual(gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gwv1beta1.ListenerStatus{
		SupportedKinds: []gwv1beta1.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))
}

func TestListenerStatusesEqual(t *testing.T) {
	t.Parallel()

	require.True(t, listenerStatusesEqual([]gwv1beta1.ListenerStatus{}, []gwv1beta1.ListenerStatus{}))
	require.False(t, listenerStatusesEqual([]gwv1beta1.ListenerStatus{}, []gwv1beta1.ListenerStatus{{}}))
	require.False(t, listenerStatusesEqual([]gwv1beta1.ListenerStatus{{
		Name: "expected",
	}}, []gwv1beta1.ListenerStatus{{
		Name: "other",
	}}))
}

func TestParentStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, parentStatusEqual(gwv1alpha2.RouteParentStatus{}, gwv1alpha2.RouteParentStatus{}))
	require.False(t, parentStatusEqual(gwv1alpha2.RouteParentStatus{}, gwv1alpha2.RouteParentStatus{
		ControllerName: "other",
	}))
	require.False(t, parentStatusEqual(gwv1alpha2.RouteParentStatus{}, gwv1alpha2.RouteParentStatus{
		ParentRef: gwv1alpha2.ParentReference{
			Name: "other",
		},
	}))
}

func TestGatewayStatusEqual(t *testing.T) {
	t.Parallel()

	require.True(t, gatewayStatusEqual(gwv1beta1.GatewayStatus{}, gwv1beta1.GatewayStatus{}))
	require.False(t, gatewayStatusEqual(gwv1beta1.GatewayStatus{}, gwv1beta1.GatewayStatus{
		Conditions: []meta.Condition{{}},
	}))
	require.False(t, gatewayStatusEqual(gwv1beta1.GatewayStatus{}, gwv1beta1.GatewayStatus{
		Listeners: []gwv1beta1.ListenerStatus{{}},
	}))
}

func TestRouteAllowedForBackendRef(t *testing.T) {
	type testCase struct {
		name        string
		fromNS      string
		toNS        *string
		toKind      *string
		toName      string
		grantFromNS string
		grantToName *string
		allowed     bool
	}

	ns1, ns2, ns3 := "namespace1", "namespace2", "namespace3"
	backend1, backend2, backend3 := "backend1", "backend2", "backend3"

	for _, tc := range []testCase{
		{name: "unspecified-backend-namespace-allowed", fromNS: ns1, toNS: nil, toName: backend1, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "same-namespace-no-name-allowed", fromNS: ns1, toNS: &ns1, toName: backend1, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "same-namespace-with-name-allowed", fromNS: ns1, toNS: &ns1, toName: backend1, grantFromNS: ns1, grantToName: &backend1, allowed: true},
		{name: "different-namespace-no-name-allowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "different-namespace-with-name-allowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns1, grantToName: &backend2, allowed: true},
		{name: "mismatched-grant-from-namespace-disallowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns3, grantToName: &backend2, allowed: false},
		{name: "mismatched-grant-to-name-disallowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns1, grantToName: &backend3, allowed: false},
	} {
		// Test each case for both HTTPRoute + TCPRoute which should function identically
		for _, routeType := range []string{"HTTPRoute", "TCPRoute"} {
			t.Run(tc.name+"-for-"+routeType, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := mocks.NewMockClient(ctrl)

				group := gwv1alpha2.Group("")

				backendRef := gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Group: &group,
						Name:  gwv1alpha2.ObjectName(tc.toName),
					},
				}

				if tc.toNS != nil {
					ns := gwv1alpha2.Namespace(*tc.toNS)
					backendRef.BackendObjectReference.Namespace = &ns
				}

				if tc.toKind != nil {
					k := gwv1alpha2.Kind(*tc.toKind)
					backendRef.Kind = &k
				}

				var route Route
				switch routeType {
				case "HTTPRoute":
					route = &gwv1alpha2.HTTPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.fromNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
						Spec: gwv1alpha2.HTTPRouteSpec{
							Rules: []gwv1alpha2.HTTPRouteRule{{
								BackendRefs: []gwv1alpha2.HTTPBackendRef{{BackendRef: backendRef}},
							}},
						},
					}
				case "TCPRoute":
					route = &gwv1alpha2.TCPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.fromNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "TCPRoute"},
						Spec: gwv1alpha2.TCPRouteSpec{
							Rules: []gwv1alpha2.TCPRouteRule{{
								BackendRefs: []gwv1alpha2.BackendRef{backendRef},
							}},
						},
					}
				default:
					require.Fail(t, fmt.Sprintf("unhandled route type %q", routeType))
				}

				var toName *gwv1alpha2.ObjectName
				if tc.grantToName != nil {
					on := gwv1alpha2.ObjectName(*tc.grantToName)
					toName = &on
				}

				if tc.toNS != nil && tc.fromNS != *tc.toNS {
					referenceGrant := gwv1alpha2.ReferenceGrant{
						TypeMeta:   meta.TypeMeta{},
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{
								Group:     "gateway.networking.k8s.io",
								Kind:      gwv1alpha2.Kind(routeType),
								Namespace: gwv1alpha2.Namespace(tc.grantFromNS),
							}},
							To: []gwv1alpha2.ReferenceGrantTo{{
								Group: "",
								Kind:  "Service",
								Name:  toName,
							}},
						},
					}

					throwawayGrant := gwv1alpha2.ReferenceGrant{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{
								Group:     "Kool & The Gang",
								Kind:      "Jungle Boogie",
								Namespace: "Wild And Peaceful",
							}},
							To: []gwv1alpha2.ReferenceGrantTo{{
								Group: "does not exist",
								Kind:  "does not exist",
								Name:  nil,
							}},
						},
					}

					client.EXPECT().
						GetReferenceGrantsInNamespace(gomock.Any(), *tc.toNS).
						Return([]gwv1alpha2.ReferenceGrant{throwawayGrant, referenceGrant}, nil)
				}

				allowed, err := routeAllowedForBackendRef(context.Background(), route, backendRef, client)
				require.NoError(t, err)
				assert.Equal(t, tc.allowed, allowed)
			})
		}
	}
}
