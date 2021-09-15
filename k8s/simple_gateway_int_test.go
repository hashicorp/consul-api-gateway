package k8s

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
)

// Test_Guide_SimpleGateway runs the documented "Simple Gateway" example
// found in the gateway-api guides[1]. It also asserts the expected
// config entries are generated and written to Consul.
//
// 1: https://gateway-api.sigs.k8s.io/v1alpha2/guides/simple-gateway/
func (suite *ControllerTestSuite) Test_Guide_SimpleGateway() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require := suite.Require()
	suite.StartController(ctx)
	gwClass := &v1alpha2.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "acme-lb",
		},
		Spec: v1alpha2.GatewayClassSpec{
			Controller: ControllerName,
		},
	}
	require.NoError(suite.Client().Create(context.Background(), gwClass))

	require.Eventually(func() bool {
		if err := suite.Client().Get(context.Background(), types.NamespacedName{Name: "acme-lb"}, gwClass); err != nil {
			return false
		}

		var admitted bool
		for _, cond := range gwClass.Status.Conditions {
			if cond.Type == string(v1alpha2.GatewayClassConditionStatusAdmitted) {
				admitted = cond.Status == metav1.ConditionTrue
			}
		}
		return admitted
	}, 5*time.Second, 100*time.Millisecond)

	routeSelectSame := v1alpha2.RouteSelectSame
	gw := &v1alpha2.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-web", Namespace: "default"},
		Spec: v1alpha2.GatewaySpec{
			GatewayClassName: gwClass.Name,
			Listeners: []v1alpha2.Listener{
				{
					Protocol: v1alpha2.HTTPProtocolType,
					Port:     v1alpha2.PortNumber(80),
					Name:     "prod-web-gw",
					Routes: &v1alpha2.ListenerRoutes{
						Namespaces: &v1alpha2.RouteNamespaces{
							From: &routeSelectSame,
						},
					},
				},
			},
		},
	}

	require.NoError(suite.Client().Create(context.Background(), gw))
	// TODO: check that Scheduled and Ready conditions eventually transition to True

	route := &v1alpha2.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1alpha2.HTTPRouteSpec{
			CommonRouteSpec: v1alpha2.CommonRouteSpec{
				ParentRefs: []v1alpha2.ParentRef{
					{
						Name: gw.Name,
					},
				},
			},
			Rules: []v1alpha2.HTTPRouteRule{
				{
					BackendRefs: []v1alpha2.HTTPBackendRef{
						{
							BackendRef: v1alpha2.BackendRef{
								BackendObjectReference: v1alpha2.BackendObjectReference{
									Name: "foo-svc",
									Port: portPtr(8080),
								},
							},
						},
					},
				},
			},
		},
	}

	require.NoError(suite.Client().Create(context.Background(), route))
	require.Eventually(func() bool {
		if err := suite.Client().Get(context.Background(), types.NamespacedName{Name: "foo", Namespace: "default"}, route); err != nil {
			return false
		}

		var admitted bool
		if len(route.Status.Parents) == 0 {
			return false
		}
		parent := route.Status.Parents[0]
		for _, cond := range parent.Conditions {
			if cond.Type == string(v1alpha2.ConditionRouteAdmitted) {
				admitted = cond.Status == metav1.ConditionTrue
			}
		}
		return admitted
	}, 10*time.Second, 1000*time.Millisecond)

	cfg := suite.Consul().ConfigEntries()
	igws, _, err := cfg.List(api.IngressGateway, nil)
	require.NoError(err)
	require.Len(igws, 1)
	igw := igws[0].(*api.IngressGatewayConfigEntry)
	require.Len(igw.Listeners, 1)
	require.Len(igw.Listeners[0].Services, 1)
	require.Equal(int(gw.Spec.Listeners[0].Port), igw.Listeners[0].Port)

	routers, _, err := cfg.List(api.ServiceRouter, nil)
	require.NoError(err)
	require.Len(routers, 1)
	router := routers[0].(*api.ServiceRouterConfigEntry)
	require.Equal(router.Name, igw.Listeners[0].Services[0].Name)
	require.Len(router.Routes, 1)
	require.NotNil(router.Routes[0].Destination)
	require.Equal(route.Spec.Rules[0].BackendRefs[0].Name, router.Routes[0].Destination.Service)
}

func portPtr(p int) *v1alpha2.PortNumber {
	v := v1alpha2.PortNumber(p)
	return &v
}
