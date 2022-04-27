package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestBinder(t *testing.T) {
	t.Parallel()

	same := gw.NamespacesFromSame
	selector := gw.NamespacesFromSelector
	other := gw.Namespace("other")
	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	udpMeta := meta.TypeMeta{}
	udpMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "UDPRoute",
	})

	for _, test := range []struct {
		name          string
		gateway       *gw.Gateway
		namespace     *core.Namespace
		listenerError error
		route         Route
		didBind       bool
	}{
		{
			name: "no match",
			gateway: &gw.Gateway{
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{}},
				},
			},
			route:   &gw.HTTPRoute{},
			didBind: false,
		},
		{
			name: "match",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{}},
				},
			},
			route: &gw.HTTPRoute{
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name: "gateway",
						}},
					},
				},
			},
			didBind: true,
		},
		{
			name: "bad route type",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Protocol: gw.HTTPProtocolType,
					}},
				},
			},
			route: &gw.UDPRoute{
				TypeMeta: udpMeta,
				Spec: gw.UDPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name: "gateway",
						}},
					},
				},
			},
			didBind: false,
		},
		{
			name: "good route type",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Protocol: gw.HTTPProtocolType,
					}},
				},
			},
			route: &gw.HTTPRoute{
				TypeMeta: routeMeta,
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name: "gateway",
						}},
					},
				},
			},
			didBind: true,
		},
		{
			name: "listener not ready",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{}},
				},
			},
			route: &gw.HTTPRoute{
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name: "gateway",
						}},
					},
				},
			},
			listenerError: errors.New("invalid"),
			didBind:       false,
		},
		{
			name: "not allowed namespace",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Name:     gw.SectionName("listener"),
						Protocol: gw.HTTPProtocolType,
						AllowedRoutes: &gw.AllowedRoutes{
							Namespaces: &gw.RouteNamespaces{
								From: &same,
							},
						},
					}},
				},
			},
			route: &gw.HTTPRoute{
				TypeMeta: routeMeta,
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name:        "gateway",
							Namespace:   &other,
							SectionName: sectionNamePtr("listener"),
						}},
					},
				},
			},
			didBind: false,
		},
		{
			name: "allowed namespace",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Name:     gw.SectionName("listener"),
						Protocol: gw.HTTPProtocolType,
						AllowedRoutes: &gw.AllowedRoutes{
							Namespaces: &gw.RouteNamespaces{
								From: &same,
							},
						},
					}},
				},
			},
			route: &gw.HTTPRoute{
				TypeMeta: routeMeta,
				ObjectMeta: meta.ObjectMeta{
					Namespace: "other",
				},
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name:        "gateway",
							Namespace:   &other,
							SectionName: sectionNamePtr("listener"),
						}},
					},
				},
			},
			didBind: true,
		},
		{
			name: "not allowed namespace match",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Name:     gw.SectionName("listener"),
						Protocol: gw.HTTPProtocolType,
						AllowedRoutes: &gw.AllowedRoutes{
							Namespaces: &gw.RouteNamespaces{
								From: &selector,
								Selector: &meta.LabelSelector{
									MatchExpressions: []meta.LabelSelectorRequirement{{
										Key:      "test",
										Operator: meta.LabelSelectorOpIn,
										Values:   []string{"foo"},
									}},
								},
							},
						},
					}},
				},
			},
			route: &gw.HTTPRoute{
				TypeMeta: routeMeta,
				ObjectMeta: meta.ObjectMeta{
					Namespace: "other",
				},
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name:        "gateway",
							Namespace:   &other,
							SectionName: sectionNamePtr("listener"),
						}},
					},
				},
			},
			namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"test": "bar",
					},
				},
			},
			didBind: false,
		},
		{
			name: "allowed namespace match",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Name:     gw.SectionName("listener"),
						Protocol: gw.HTTPProtocolType,
						AllowedRoutes: &gw.AllowedRoutes{
							Namespaces: &gw.RouteNamespaces{
								From: &selector,
								Selector: &meta.LabelSelector{
									MatchExpressions: []meta.LabelSelectorRequirement{{
										Key:      "test",
										Operator: meta.LabelSelectorOpIn,
										Values:   []string{"foo"},
									}},
								},
							},
						},
					}},
				},
			},
			route: &gw.HTTPRoute{
				TypeMeta: routeMeta,
				ObjectMeta: meta.ObjectMeta{
					Namespace: "other",
				},
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name:        "gateway",
							Namespace:   &other,
							SectionName: sectionNamePtr("listener"),
						}},
					},
				},
			},
			namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"test": "foo",
					},
				},
			},
			didBind: true,
		},
		{
			name: "hostname no match",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Name:     gw.SectionName("listener"),
						Hostname: hostnamePtr("host"),
					}},
				},
			},
			route: &gw.HTTPRoute{
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name:        "gateway",
							SectionName: sectionNamePtr("listener"),
						}},
					},
					Hostnames: []gw.Hostname{"other"},
				},
			},
			didBind: false,
		},
		{
			name: "hostname match",
			gateway: &gw.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gw.GatewaySpec{
					Listeners: []gw.Listener{{
						Name:     gw.SectionName("listener"),
						Hostname: hostnamePtr("host"),
					}},
				},
			},
			route: &gw.HTTPRoute{
				Spec: gw.HTTPRouteSpec{
					CommonRouteSpec: gw.CommonRouteSpec{
						ParentRefs: []gw.ParentRef{{
							Name:        "gateway",
							SectionName: sectionNamePtr("listener"),
						}},
					},
					Hostnames: []gw.Hostname{"other", "host"},
				},
			},
			didBind: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mocks.NewMockClient(ctrl)

			gatewayState := state.InitialGatewayState("", test.gateway)
			if test.listenerError != nil {
				gatewayState.Listeners[0].Status.Ready.Invalid = test.listenerError
			}
			if test.namespace != nil {
				client.EXPECT().GetNamespace(gomock.Any(), gomock.Any()).Return(test.namespace, nil)
			}

			binder := NewBinder(client)
			gateway := NewGateway(v1alpha1.GatewayClassConfig{}, test.gateway, gatewayState)
			route := NewRoute(test.route, state.NewRouteState())
			bound, err := binder.Bind(context.Background(), gateway, route)
			require.NoError(t, err)
			if test.didBind {
				require.True(t, bound)
			} else {
				require.False(t, bound)
			}
		})
	}
}

func TestRouteAllowedForListenerNamespaces(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	// same
	same := gw.NamespacesFromSame

	allowed, err := routeAllowedForListenerNamespaces(context.Background(), "expected", &gw.AllowedRoutes{
		Namespaces: &gw.RouteNamespaces{
			From: &same,
		},
	}, NewRoute(&gw.HTTPRoute{
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
	}, NewRoute(&gw.HTTPRoute{
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
	}, NewRoute(&gw.HTTPRoute{
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
	}, NewRoute(&gw.HTTPRoute{
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
	}, NewRoute(&gw.HTTPRoute{
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
	}, NewRoute(&gw.HTTPRoute{
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
	}, NewRoute(&gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "expected",
		},
	}, state.NewRouteState()), client)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestRouteKindIsAllowedForListener(t *testing.T) {
	t.Parallel()

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	require.True(t, routeKindIsAllowedForListener([]gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "HTTPRoute",
	}}, NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
	}, state.NewRouteState())))
	require.False(t, routeKindIsAllowedForListener([]gw.RouteGroupKind{{
		Group: (*gw.Group)(&gw.GroupVersion.Group),
		Kind:  "TCPRoute",
	}}, NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
	}, state.NewRouteState())))
}

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

func sectionNamePtr(name string) *gw.SectionName {
	value := gw.SectionName(name)
	return &value
}

func hostnamePtr(name string) *gw.Hostname {
	value := gw.Hostname(name)
	return &value
}
