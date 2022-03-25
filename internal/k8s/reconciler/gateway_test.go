package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	"github.com/hashicorp/go-hclog"
)

func TestGatewayID(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			},
		},
		ConsulNamespace: "consul",
	})
	require.Equal(t, internalCore.GatewayID{Service: "name", ConsulNamespace: "consul"}, gateway.ID())
}

func TestGatewayMeta(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			ObjectMeta: meta.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			},
		},
		ConsulNamespace: "consul",
	})
	require.NotNil(t, gateway.Meta())
}

func TestGatewayListeners(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{}},
			},
		},
	})
	require.Len(t, gateway.Listeners(), 1)
}

func TestGatewayTrackSync(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	gateway := factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	gateway.Gateway.Status = gateway.GetStatus(gateway.Gateway)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	expected := errors.New("expected")

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	require.True(t, errors.Is(gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}), expected))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(expected)
	require.Equal(t, expected, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return true, nil
	}))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.Gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, expected
	}))
}

func TestGatewayShouldBind(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway := factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Protocol: gw.HTTPProtocolType,
			}},
		},
	}})
	gateway.Gateway.Name = "name"

	require.Empty(t, gateway.Bind(context.Background(), storeMocks.NewMockRoute(nil)))

	route := factory.NewRoute(&gw.HTTPRoute{})
	route.RouteState.ResolutionErrors.Add(service.NewConsulResolutionError("test"))
	require.Empty(t, gateway.Bind(context.Background(), route))

	require.NotEmpty(t, gateway.Bind(context.Background(), factory.NewRoute(&gw.HTTPRoute{
		TypeMeta: meta.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/something",
		},
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "name",
				}},
			},
		},
	})))

	require.Empty(t, gateway.Bind(context.Background(), factory.NewRoute(&gw.HTTPRoute{})))
}

func serviceType(v core.ServiceType) *core.ServiceType {
	return &v
}
