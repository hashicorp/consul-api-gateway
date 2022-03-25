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
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/go-hclog"
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

			factory := NewFactory(FactoryConfig{
				Logger: hclog.NewNullLogger(),
			})
			state := state.InitialGatewayState(test.gateway)
			if test.listenerError != nil {
				state.Listeners[0].Status.Ready.Invalid = test.listenerError
			}
			if test.namespace != nil {
				client.EXPECT().GetNamespace(gomock.Any(), gomock.Any()).Return(test.namespace, nil)
			}

			binder := NewBinder(client, test.gateway, state)
			listeners := binder.Bind(context.Background(), factory.NewRoute(test.route))
			if test.didBind {
				require.NotEmpty(t, listeners)
			} else {
				require.Empty(t, listeners)
			}
		})
	}
}

func sectionNamePtr(name string) *gw.SectionName {
	value := gw.SectionName(name)
	return &value
}

func hostnamePtr(name string) *gw.Hostname {
	value := gw.Hostname(name)
	return &value
}
