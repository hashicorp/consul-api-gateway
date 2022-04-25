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
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestRouteMatchesListener(t *testing.T) {
	t.Parallel()

	name := gw.SectionName("name")
	can, must := routeMatchesListener(name, &name)
	require.True(t, can)
	require.True(t, must)

	can, must = routeMatchesListener(name, nil)
	require.True(t, can)
	require.False(t, must)

	can, must = routeMatchesListener(gw.SectionName("other"), &name)
	require.False(t, can)
	require.True(t, must)
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

func TestRouteKindIsAllowedForListener(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	require.True(t, routeKindIsAllowedForListener([]gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "HTTPRoute",
	}}, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
	}, state.NewRouteState())))
	require.False(t, routeKindIsAllowedForListener([]gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "TCPRoute",
	}}, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
	}, state.NewRouteState())))
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
	same := gw.NamespacesFromSame

	allowed, err := routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, state.NewRouteState()), client)
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}, state.NewRouteState()), client)
	require.NoError(t, err)
	require.False(t, allowed)

	// all
	all := gw.NamespacesFromAll
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &all,
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "other",
		},
	}, state.NewRouteState()), client)
	require.NoError(t, err)
	require.True(t, allowed)

	// selector
	selector := gw.NamespacesFromSelector

	matchingNamespace := &core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Labels: map[string]string{
				"label":                       "test",
				"kubernetes.io/metadata.name": "expected",
			}}}
	invalidNamespace := &core.Namespace{ObjectMeta: meta.ObjectMeta{Labels: map[string]string{}}}

	client.EXPECT().GetNamespace(context.Background(), types.NamespacedName{Name: "expected"}).Return(invalidNamespace, nil).Times(1)
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, state.NewRouteState()), client)
	require.NoError(t, err)
	require.False(t, allowed)

	client.EXPECT().GetNamespace(context.Background(), types.NamespacedName{Name: "expected"}).Return(matchingNamespace, nil).Times(1)
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label": "test",
				},
			},
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, state.NewRouteState()), client)
	require.NoError(t, err)
	require.True(t, allowed)

	_, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &selector,
			Selector: &meta.LabelSelector{
				MatchExpressions: []meta.LabelSelectorRequirement{{
					Key:      "test",
					Operator: meta.LabelSelectorOperator("invalid"),
				}},
			},
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, state.NewRouteState()), client)
	require.Error(t, err)

	// unknown
	unknown := gw.FromNamespaces("unknown")
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &unknown,
		},
	}, factory.NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, state.NewRouteState()), client)
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

	require.True(t, listenerStatusEqual(gw.ListenerStatus{}, gw.ListenerStatus{}))
	require.False(t, listenerStatusEqual(gw.ListenerStatus{
		Name: "expected",
	}, gw.ListenerStatus{
		Name: "other",
	}))
	require.False(t, listenerStatusEqual(gw.ListenerStatus{
		AttachedRoutes: 1,
	}, gw.ListenerStatus{
		AttachedRoutes: 2,
	}))

	groupOne := gw.Group("group")
	kindOne := gw.Kind("kind")
	groupTwo := gw.Group("group")
	kindTwo := gw.Kind("kind")
	require.True(t, listenerStatusEqual(gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))

	groupTwo = gw.Group("other")
	require.False(t, listenerStatusEqual(gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupOne,
			Kind:  kindOne,
		}},
	}, gw.ListenerStatus{
		SupportedKinds: []gw.RouteGroupKind{{
			Group: &groupTwo,
			Kind:  kindTwo,
		}},
	}))
}

func TestRouteAllowedForBackendRef(t *testing.T) {
	type testCase struct {
		name         string
		routeNS      string
		backendNS    *string
		backendKind  *string
		backendName  string
		policyFromNS string
		policyToName *string
		allowed      bool
	}

	ns1, ns2, ns3 := "namespace1", "namespace2", "namespace3"
	backend1, backend2, backend3 := "backend1", "backend2", "backend3"

	for _, tc := range []testCase{
		{name: "unspecified-backend-namespace-allowed", routeNS: ns1, backendNS: nil, backendName: backend1, policyFromNS: ns1, policyToName: nil, allowed: true},
		{name: "same-namespace-no-name-allowed", routeNS: ns1, backendNS: &ns1, backendName: backend1, policyFromNS: ns1, policyToName: nil, allowed: true},
		{name: "same-namespace-with-name-allowed", routeNS: ns1, backendNS: &ns1, backendName: backend1, policyFromNS: ns1, policyToName: &backend1, allowed: true},
		{name: "different-namespace-no-name-allowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns1, policyToName: nil, allowed: true},
		{name: "different-namespace-with-name-allowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns1, policyToName: &backend2, allowed: true},
		{name: "mismatched-policy-from-namespace-disallowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns3, policyToName: &backend2, allowed: false},
		{name: "mismatched-policy-to-name-disallowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns1, policyToName: &backend3, allowed: false},
	} {
		// Test each case for both HTTPRoute + TCPRoute which should function identically
		for _, routeType := range []string{"HTTPRoute", "TCPRoute"} {
			t.Run(tc.name+"-for-"+routeType, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := mocks.NewMockClient(ctrl)

				group := gw.Group("")

				backendRef := gw.BackendRef{
					BackendObjectReference: gw.BackendObjectReference{
						Group: &group,
						Name:  gw.ObjectName(tc.backendName),
					},
				}

				if tc.backendNS != nil {
					ns := gw.Namespace(*tc.backendNS)
					backendRef.BackendObjectReference.Namespace = &ns
				}

				if tc.backendKind != nil {
					k := gw.Kind(*tc.backendKind)
					backendRef.Kind = &k
				}

				var route Route
				switch routeType {
				case "HTTPRoute":
					route = &gw.HTTPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.routeNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
						Spec: gw.HTTPRouteSpec{
							Rules: []gw.HTTPRouteRule{{
								BackendRefs: []gw.HTTPBackendRef{{BackendRef: backendRef}},
							}},
						},
					}
				case "TCPRoute":
					route = &gw.TCPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.routeNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "TCPRoute"},
						Spec: gw.TCPRouteSpec{
							Rules: []gw.TCPRouteRule{{
								BackendRefs: []gw.BackendRef{backendRef},
							}},
						},
					}
				default:
					require.Fail(t, fmt.Sprintf("unhandled route type %q", routeType))
				}

				var toName *gw.ObjectName
				if tc.policyToName != nil {
					on := gw.ObjectName(*tc.policyToName)
					toName = &on
				}

				if tc.backendNS != nil && tc.routeNS != *tc.backendNS {
					referencePolicy := gw.ReferencePolicy{
						TypeMeta:   meta.TypeMeta{},
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.backendNS},
						Spec: gw.ReferencePolicySpec{
							From: []gw.ReferencePolicyFrom{{
								Group:     "gateway.networking.k8s.io",
								Kind:      gw.Kind(routeType),
								Namespace: gw.Namespace(tc.policyFromNS),
							}},
							To: []gw.ReferencePolicyTo{{
								Group: "",
								Kind:  "Service",
								Name:  toName,
							}},
						},
					}

					throwawayPolicy := gw.ReferencePolicy{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.backendNS},
						Spec: gw.ReferencePolicySpec{
							From: []gw.ReferencePolicyFrom{{
								Group:     "Kool & The Gang",
								Kind:      "Jungle Boogie",
								Namespace: "Wild And Peaceful",
							}},
							To: []gw.ReferencePolicyTo{{
								Group: "does not exist",
								Kind:  "does not exist",
								Name:  nil,
							}},
						},
					}

					client.EXPECT().
						GetReferencePoliciesInNamespace(gomock.Any(), *tc.backendNS).
						Return([]gw.ReferencePolicy{throwawayPolicy, referencePolicy}, nil)
				}

				allowed, err := routeAllowedForBackendRef(context.Background(), route, backendRef, client)
				require.NoError(t, err)
				assert.Equal(t, tc.allowed, allowed)
			})
		}
	}
}
