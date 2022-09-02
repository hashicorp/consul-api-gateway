package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
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
	gateway.Gateway.Status = gateway.GatewayState.GetStatus(gw)
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
