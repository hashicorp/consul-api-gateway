package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGatewayID(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gw := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}

	gwState := state.InitialGatewayState(gw)
	gwState.ConsulNamespace = "consul"

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           gwState,
		ConsulNamespace: "consul",
	})
	require.Equal(t, internalCore.GatewayID{Service: "name", ConsulNamespace: "consul"}, gateway.ID())
}

func TestGatewayMeta(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gw := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	require.NotNil(t, gateway.Meta())
}

func TestGatewayListeners(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gw := &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{}},
		},
	}
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	require.Len(t, gateway.Listeners(), 1)
}

func TestGatewayOutputStatus(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	// Pending listener
	gw := &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
		},
	}

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	gateway.GatewayState.Addresses = []string{"127.0.0.1"}
	gateway.listeners[0].status.Ready.Pending = errors.New("pending")
	require.Len(t, gateway.Status().Addresses, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonListenersNotReady, gateway.GatewayState.Status.Ready.Condition(0).Reason)

	// Service ready, pods not
	gw = &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
		},
	}

	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	gateway.GatewayState.PodReady = false
	gateway.GatewayState.ServiceReady = true
	gateway.listeners[0].status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonListenersNotValid, gateway.GatewayState.Status.Ready.Condition(0).Reason)

	// Pods ready, service not
	gw = &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
		},
	}

	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	gateway.GatewayState.PodReady = true
	gateway.GatewayState.ServiceReady = false
	gateway.listeners[0].status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonListenersNotValid, gateway.GatewayState.Status.Ready.Condition(0).Reason)

	// Pods + service ready
	gw = &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
			Addresses: []gwv1beta1.GatewayAddress{{}},
		},
	}

	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	gateway.GatewayState.PodReady = true
	gateway.GatewayState.ServiceReady = true
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonAddressNotAssigned, gateway.GatewayState.Status.Ready.Condition(0).Reason)

	gw = &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
			Addresses: []gwv1beta1.GatewayAddress{{}},
		},
	}

	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	gateway.Gateway.Status = gateway.Status()
	gateway.Status()
}

func TestGatewayTrackSync(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
		Deployer: NewDeployer(DeployerConfig{
			Logger: hclog.NewNullLogger(),
			Client: client,
		}),
	})

	gw := &gwv1beta1.Gateway{}

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	gateway.Gateway.Status = gateway.Status()
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gw = &gwv1beta1.Gateway{}

	gateway = factory.NewGateway(NewGatewayConfig{
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				DeploymentSpec: apigwv1alpha1.DeploymentSpec{
					DefaultInstances: pointer.Int32(2),
				},
			},
		},
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})

	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	expected := errors.New("expected")

	gw = &gwv1beta1.Gateway{}
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	require.True(t, errors.Is(gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}), expected))

	gw = &gwv1beta1.Gateway{}
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(expected)
	require.Equal(t, expected, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gw = &gwv1beta1.Gateway{}
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return true, nil
	}))

	gw = &gwv1beta1.Gateway{}
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		State:           state.InitialGatewayState(gw),
		ConsulNamespace: "consul",
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, expected
	}))
}

func TestGatewayShouldUpdate(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gw := &gwv1beta1.Gateway{}
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		ConsulNamespace: "consul",
	})

	otherGW := &gwv1beta1.Gateway{}
	other := factory.NewGateway(NewGatewayConfig{
		Gateway:         otherGW,
		ConsulNamespace: "consul",
	})

	// Have equal resource version
	gateway.Gateway.ObjectMeta.ResourceVersion = `0`
	other.Gateway.ObjectMeta.ResourceVersion = `0`
	assert.True(t, gateway.ShouldUpdate(other))

	// Have greater resource version
	gateway.Gateway.ObjectMeta.ResourceVersion = `1`
	other.Gateway.ObjectMeta.ResourceVersion = `0`
	assert.False(t, gateway.ShouldUpdate(other))

	// Have lesser resource version
	gateway.Gateway.ObjectMeta.ResourceVersion = `0`
	other.Gateway.ObjectMeta.ResourceVersion = `1`
	assert.True(t, gateway.ShouldUpdate(other))

	// Have non-numeric resource version
	gateway.Gateway.ObjectMeta.ResourceVersion = `a`
	other.Gateway.ObjectMeta.ResourceVersion = `0`
	assert.True(t, gateway.ShouldUpdate(other))

	// Other gateway non-numeric resource version
	gateway.Gateway.ObjectMeta.ResourceVersion = `0`
	other.Gateway.ObjectMeta.ResourceVersion = `a`
	assert.False(t, gateway.ShouldUpdate(other))

	// Other gateway nil
	assert.False(t, gateway.ShouldUpdate(nil))

	// Have nil gateway
	gateway = nil
	assert.True(t, gateway.ShouldUpdate(other))
}

func TestGatewayShouldBind(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gw := &gwv1beta1.Gateway{}
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway:         gw,
		ConsulNamespace: "consul",
	})
	gateway.Gateway.Name = "name"

	require.False(t, gateway.ShouldBind(storeMocks.NewMockRoute(nil)))

	route := newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	route.RouteState.ResolutionErrors.Add(service.NewConsulResolutionError("test"))
	require.False(t, gateway.ShouldBind(route))

	require.True(t, gateway.ShouldBind(newK8sRoute(&gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name: "name",
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))

	require.False(t, gateway.ShouldBind(newK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))
}
