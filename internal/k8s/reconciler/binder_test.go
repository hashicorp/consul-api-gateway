package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/go-hclog"
)

func TestBinder(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	// no match
	listener := NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{}), false)

	// match
	listener = NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
		},
	}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "gateway",
				}},
			},
		},
	}), true)

	// not ready
	listener = NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
		},
	}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	listener.ListenerState.Status.Ready.Invalid = errors.New("invalid")

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "gateway",
				}},
			},
		},
	}), false)
}

func TestBinder_RouteKind(t *testing.T) {
	t.Parallel()

	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "UDPRoute",
	})
	name := gw.SectionName("listener")

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	listener := NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
		},
	}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	validator := NewGatewayValidator(nil)
	validator.validateProtocols(listener.ListenerState, listener.listener)
	testBinder(t, listener, factory.NewRoute(&gw.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "gateway",
				}},
			},
		},
	}), false)

	listener = NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	testBinder(t, listener, factory.NewRoute(&gw.UDPRoute{
		TypeMeta: routeMeta,
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:        "gateway",
					SectionName: &name,
				}},
			},
		},
	}), false)
}

func TestBinder_AllowedNamespaces(t *testing.T) {
	t.Parallel()

	name := gw.SectionName("listener")
	same := gw.NamespacesFromSame
	selector := gw.NamespacesFromSelector
	other := gw.Namespace("other")
	routeMeta := meta.TypeMeta{}
	routeMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gw.GroupVersion.Group,
		Version: gw.GroupVersion.Version,
		Kind:    "HTTPRoute",
	})

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	listener := NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name:      "gateway",
				Namespace: "other",
			},
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Namespaces: &gw.RouteNamespaces{
				From: &same,
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:        "gateway",
					Namespace:   &other,
					SectionName: &name,
				}},
			},
		},
	}), false)

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:      "gateway",
					Namespace: &other,
				}},
			},
		},
	}), false)

	listener = NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name:      "gateway",
				Namespace: "other",
			},
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Namespaces: &gw.RouteNamespaces{
				From: &selector,
				Selector: &meta.LabelSelector{
					MatchExpressions: []meta.LabelSelectorRequirement{{
						Key:      "test",
						Operator: meta.LabelSelectorOperator("invalid"),
					}},
				},
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:        "gateway",
					Namespace:   &other,
					SectionName: &name,
				}},
			},
		},
	}), false)

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:      "gateway",
					Namespace: &other,
				}},
			},
		},
	}), false)
}

func TestBinder_HostnameMatch(t *testing.T) {
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
	name := gw.SectionName("listener")
	hostname := gw.Hostname("hostname")

	listener := NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
		},
	}, gw.Listener{
		Name:     name,
		Hostname: &hostname,
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:        "gateway",
					SectionName: &name,
				}},
			},
			Hostnames: []gw.Hostname{"other"},
		},
	}), false)

	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: routeMeta,
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "gateway",
				}},
			},
			Hostnames: []gw.Hostname{"other"},
		},
	}), false)
}

func TestBinder_NameMatch(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	name := gw.SectionName("listener")
	otherName := gw.SectionName("other")
	listener := NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name: "gateway",
			},
		},
	}, gw.Listener{
		Name:     name,
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	testBinder(t, listener, factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name:        "gateway",
					SectionName: &otherName,
				}},
			},
			Hostnames: []gw.Hostname{"other"},
		},
	}), false)
}

func testBinder(t *testing.T, listener *K8sListener, route *K8sRoute, expected bool) {
	t.Helper()

	binder := &Binder{
		Client:        listener.client,
		Gateway:       listener.gateway,
		Listener:      listener.listener,
		ListenerState: listener.ListenerState,
	}
	require.Equal(t, expected, binder.Bind(context.Background(), route))
}
