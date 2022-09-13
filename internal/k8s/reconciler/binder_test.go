package reconciler

import (
	"context"
	"errors"
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
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
)

func TestBinder(t *testing.T) {
	t.Parallel()

	same := gwv1beta1.NamespacesFromSame
	selector := gwv1beta1.NamespacesFromSelector
	other := gwv1alpha2.Namespace("other")
	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1alpha2.GroupVersion.Group,
		Version: gwv1alpha2.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})
	udpMeta := meta.TypeMeta{}
	udpMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gwv1alpha2.GroupVersion.Group,
		Version: gwv1alpha2.GroupVersion.Version,
		Kind:    "UDPRoute",
	})

	for _, test := range []struct {
		name          string
		gateway       *gwv1beta1.Gateway
		namespace     *core.Namespace
		listenerError error
		route         Route
		didBind       bool
	}{
		{
			name: "no match",
			gateway: &gwv1beta1.Gateway{
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{}},
				},
			},
			route:   &gwv1alpha2.HTTPRoute{},
			didBind: false,
		},
		{
			name: "match",
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: "gateway",
						}},
					},
				},
			},
			didBind: true,
		},
		{
			name: "bad route type",
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Protocol: gwv1beta1.HTTPProtocolType,
					}},
				},
			},
			route: &gwv1alpha2.UDPRoute{
				TypeMeta: udpMeta,
				Spec: gwv1alpha2.UDPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: "gateway",
						}},
					},
				},
			},
			didBind: false,
		},
		{
			name: "good route type",
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Protocol: gwv1beta1.HTTPProtocolType,
					}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				TypeMeta: routeMeta,
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: "gateway",
						}},
					},
				},
			},
			didBind: true,
		},
		{
			name: "listener not ready",
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
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
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Name:     gwv1beta1.SectionName("listener"),
						Protocol: gwv1beta1.HTTPProtocolType,
						AllowedRoutes: &gwv1beta1.AllowedRoutes{
							Namespaces: &gwv1beta1.RouteNamespaces{
								From: &same,
							},
						},
					}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				TypeMeta: routeMeta,
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
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
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Name:     gwv1beta1.SectionName("listener"),
						Protocol: gwv1beta1.HTTPProtocolType,
						AllowedRoutes: &gwv1beta1.AllowedRoutes{
							Namespaces: &gwv1beta1.RouteNamespaces{
								From: &same,
							},
						},
					}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				TypeMeta: routeMeta,
				ObjectMeta: meta.ObjectMeta{
					Namespace: "other",
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
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
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Name:     gwv1beta1.SectionName("listener"),
						Protocol: gwv1beta1.HTTPProtocolType,
						AllowedRoutes: &gwv1beta1.AllowedRoutes{
							Namespaces: &gwv1beta1.RouteNamespaces{
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
			route: &gwv1alpha2.HTTPRoute{
				TypeMeta: routeMeta,
				ObjectMeta: meta.ObjectMeta{
					Namespace: "other",
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
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
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      "gateway",
					Namespace: "other",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Name:     gwv1beta1.SectionName("listener"),
						Protocol: gwv1beta1.HTTPProtocolType,
						AllowedRoutes: &gwv1beta1.AllowedRoutes{
							Namespaces: &gwv1beta1.RouteNamespaces{
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
			route: &gwv1alpha2.HTTPRoute{
				TypeMeta: routeMeta,
				ObjectMeta: meta.ObjectMeta{
					Namespace: "other",
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
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
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Name:     gwv1beta1.SectionName("listener"),
						Hostname: hostnamePtr("host"),
					}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name:        "gateway",
							SectionName: sectionNamePtr("listener"),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"other"},
				},
			},
			didBind: false,
		},
		{
			name: "hostname match",
			gateway: &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name: "gateway",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						Name:     gwv1beta1.SectionName("listener"),
						Hostname: hostnamePtr("host"),
					}},
				},
			},
			route: &gwv1alpha2.HTTPRoute{
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name:        "gateway",
							SectionName: sectionNamePtr("listener"),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"other", "host"},
				},
			},
			didBind: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mocks.NewMockClient(ctrl)

			factory := NewFactory(FactoryConfig{
				Logger: hclog.NewNullLogger(),
			})
			gatewayState := state.InitialGatewayState(test.gateway)
			if test.listenerError != nil {
				gatewayState.Listeners[0].Status.Ready.Invalid = test.listenerError
			}
			if test.namespace != nil {
				client.EXPECT().GetNamespace(gomock.Any(), gomock.Any()).Return(test.namespace, nil)
			}

			binder := NewBinder(client)
			listeners := binder.Bind(context.Background(),
				factory.NewGateway(NewGatewayConfig{Gateway: test.gateway, State: gatewayState}),
				factory.NewRoute(NewRouteConfig{Route: test.route}))
			if test.didBind {
				require.NotEmpty(t, listeners)
			} else {
				require.Empty(t, listeners)
			}
		})
	}
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
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "expected",
			},
		},
	}), client)
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &same,
		},
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "other",
			},
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
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "other",
			},
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
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "expected",
			},
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
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "expected",
			},
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
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "expected",
			},
		},
	}), client)
	require.Error(t, err)

	// unknown
	unknown := gwv1beta1.FromNamespaces("unknown")
	allowed, err = routeAllowedForListenerNamespaces(context.Background(), "expected", &gwv1beta1.AllowedRoutes{
		Namespaces: &gwv1beta1.RouteNamespaces{
			From: &unknown,
		},
	}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			ObjectMeta: meta.ObjectMeta{
				Namespace: "expected",
			},
		},
	}), client)
	require.NoError(t, err)
	require.False(t, allowed)
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
	require.True(t, routeKindIsAllowedForListener([]gwv1beta1.RouteGroupKind{{
		Group: (*gwv1beta1.Group)(&gwv1alpha2.GroupVersion.Group),
		Kind:  "HTTPRoute",
	}}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			TypeMeta: routeMeta,
		},
	})))
	require.False(t, routeKindIsAllowedForListener([]gwv1beta1.RouteGroupKind{{
		Group: (*gwv1beta1.Group)(&gwv1alpha2.GroupVersion.Group),
		Kind:  "TCPRoute",
	}}, factory.NewRoute(NewRouteConfig{
		Route: &gwv1alpha2.HTTPRoute{
			TypeMeta: routeMeta,
		},
	})))
}

func TestRouteMatchesListener(t *testing.T) {
	t.Parallel()

	name := gwv1alpha2.SectionName("name")
	can, must := routeMatchesListener("name", &name)
	assert.True(t, can)
	assert.True(t, must)

	can, must = routeMatchesListener("name", nil)
	assert.True(t, can)
	assert.False(t, must)

	can, must = routeMatchesListener("other", &name)
	assert.False(t, can)
	assert.True(t, must)
}

func sectionNamePtr(name string) *gwv1alpha2.SectionName {
	value := gwv1alpha2.SectionName(name)
	return &value
}

func hostnamePtr(name string) *gwv1beta1.Hostname {
	value := gwv1beta1.Hostname(name)
	return &value
}
