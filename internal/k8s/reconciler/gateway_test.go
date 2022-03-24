package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

func TestGatewayValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})

	hostname := gw.Hostname("*")
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Hostname: &hostname,
					Protocol: gw.HTTPSProtocolType,
					TLS: &gw.GatewayTLSConfig{
						CertificateRefs: []*gw.SecretObjectReference{{}},
					},
				}},
			},
		},
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))

	expected := errors.New("expected")
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.True(t, errors.Is(gateway.Validate(context.Background()), expected))

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected).Times(1)
	require.True(t, errors.Is(gateway.Validate(context.Background()), expected))
}

func TestGatewayValidateGatewayIP(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})

	hostname := gw.Hostname("*")

	gwDef := &gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Hostname: &hostname,
				Protocol: gw.HTTPSProtocolType,
				TLS: &gw.GatewayTLSConfig{
					CertificateRefs: []*gw.SecretObjectReference{{}},
				},
			}},
		},
	}

	pod := &core.Pod{
		Status: core.PodStatus{
			HostIP: "1.1.1.1",
			PodIP:  "2.2.2.2",
		},
	}

	svc := &core.Service{
		Spec: core.ServiceSpec{
			ClusterIP: "3.3.3.3",
		},
		Status: core.ServiceStatus{
			LoadBalancer: core.LoadBalancerStatus{
				Ingress: []core.LoadBalancerIngress{
					{
						IP: "4.4.4.4",
					},
				},
			},
		},
	}

	for _, tc := range []struct {
		// What IP address do we expect the Gateway to be assigned?
		expectedIP string

		// Should the mock client expect a request for the Service?
		// If false, the mock client expects a request for the Pod instead.
		expectedIPFromSvc bool

		// What serviceType should the gateway be configured for?
		serviceType *core.ServiceType
	}{
		{
			expectedIP:        pod.Status.PodIP,
			expectedIPFromSvc: false,
			serviceType:       nil,
		},
		{
			expectedIP:        pod.Status.HostIP,
			expectedIPFromSvc: false,
			serviceType:       serviceType(core.ServiceTypeNodePort),
		},
		{
			expectedIP:        svc.Status.LoadBalancer.Ingress[0].IP,
			expectedIPFromSvc: true,
			serviceType:       serviceType(core.ServiceTypeLoadBalancer),
		},
		{
			expectedIP:        svc.Spec.ClusterIP,
			expectedIPFromSvc: true,
			serviceType:       serviceType(core.ServiceTypeClusterIP),
		},
	} {
		name := "Service type <nil>"
		if tc.serviceType != nil {
			name = fmt.Sprintf("Service type %s", *tc.serviceType)
		}

		t.Run(name, func(t *testing.T) {
			gateway := factory.NewGateway(NewGatewayConfig{
				Gateway: gwDef,
				Config: apigwv1alpha1.GatewayClassConfig{
					Spec: apigwv1alpha1.GatewayClassConfigSpec{
						ServiceType: tc.serviceType,
					},
				},
			})

			if tc.expectedIPFromSvc {
				client.EXPECT().GetService(gomock.Any(), gomock.Any()).Return(svc, nil)
			} else {
				client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(pod, nil)
			}
			assert.NoError(t, gateway.validateGatewayIP(context.Background()))

			require.Len(t, gateway.addresses, 1)
			assert.Equal(t, tc.expectedIP, gateway.addresses[0])

			assert.True(t, gateway.serviceReady)
		})
	}
}

func TestGatewayValidate_ListenerProtocolConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name:     gw.SectionName("1"),
					Protocol: gw.HTTPProtocolType,
					Port:     gw.PortNumber(1),
				}, {
					Name:     gw.SectionName("2"),
					Protocol: gw.UDPProtocolType,
					Port:     gw.PortNumber(1),
				}},
			},
		},
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
	})
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, ListenerConditionReasonProtocolConflict, gateway.listeners["1"].ListenerState.Status.Conflicted.Condition(0).Reason)
	require.Equal(t, ListenerConditionReasonProtocolConflict, gateway.listeners["2"].ListenerState.Status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_ListenerHostnameConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})

	hostname := gw.Hostname("1")
	other := gw.Hostname("2")
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name:     gw.SectionName("1"),
					Protocol: gw.HTTPProtocolType,
					Hostname: &hostname,
					Port:     gw.PortNumber(1),
				}, {
					Name:     gw.SectionName("2"),
					Protocol: gw.HTTPProtocolType,
					Hostname: &other,
					Port:     gw.PortNumber(1),
				}},
			},
		},
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
	})
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, ListenerConditionReasonHostnameConflict, gateway.listeners["1"].ListenerState.Status.Conflicted.Condition(0).Reason)
	require.Equal(t, ListenerConditionReasonHostnameConflict, gateway.listeners["2"].ListenerState.Status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_Pods(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})

	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{}},
			},
		},
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
	})

	// Pod has no/unknown status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{},
	}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonUnknown, gateway.status.Scheduled.Condition(0).Reason)

	// Pod has pending status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonNotReconciled, gateway.status.Scheduled.Condition(0).Reason)

	// Pod is marked as unschedulable
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodPending,
			Conditions: []core.PodCondition{{
				Type:   core.PodScheduled,
				Status: core.ConditionFalse,
				Reason: "Unschedulable",
			}},
		},
	}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.Equal(t, GatewayConditionReasonNoResources, gateway.status.Scheduled.Condition(0).Reason)

	// Pod has running status and is marked ready
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodRunning,
			Conditions: []core.PodCondition{{
				Type:   core.PodReady,
				Status: core.ConditionTrue,
			}},
		},
	}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.True(t, gateway.podReady)

	// Pod has succeeded status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
		},
	}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.Equal(t, GatewayConditionReasonPodFailed, gateway.status.Scheduled.Condition(0).Reason)

	// Pod has failed status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodFailed,
		},
	}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.Equal(t, GatewayConditionReasonPodFailed, gateway.status.Scheduled.Condition(0).Reason)
}

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

func TestGatewayOutputStatus(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	// Pending listener
	gateway := factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name: gw.SectionName("1"),
				}},
			},
		},
	})
	gateway.addresses = []string{"127.0.0.1"}
	gateway.listeners["1"].ListenerState.Status.Ready.Pending = errors.New("pending")
	require.Len(t, gateway.Status().Addresses, 1)
	assert.Equal(t, GatewayConditionReasonListenersNotReady, gateway.status.Ready.Condition(0).Reason)

	// Service ready, pods not
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name: gw.SectionName("1"),
				}},
			},
		},
	})
	gateway.podReady = false
	gateway.serviceReady = true
	gateway.listeners["1"].ListenerState.Status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, GatewayConditionReasonListenersNotValid, gateway.status.Ready.Condition(0).Reason)

	// Pods ready, service not
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name: gw.SectionName("1"),
				}},
			},
		},
	})
	gateway.podReady = true
	gateway.serviceReady = false
	gateway.listeners["1"].ListenerState.Status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, GatewayConditionReasonListenersNotValid, gateway.status.Ready.Condition(0).Reason)

	// Pods + service ready
	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name: gw.SectionName("1"),
				}},
				Addresses: []gw.GatewayAddress{{}},
			},
		},
	})
	gateway.podReady = true
	gateway.serviceReady = true
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, GatewayConditionReasonAddressNotAssigned, gateway.status.Ready.Condition(0).Reason)

	gateway = factory.NewGateway(NewGatewayConfig{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Listeners: []gw.Listener{{
					Name: gw.SectionName("1"),
				}},
				Addresses: []gw.GatewayAddress{{}},
			},
		},
	})
	gateway.gateway.Status = gateway.Status()
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
	})
	gateway := factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	gateway.gateway.Status = gateway.Status()
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
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
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(expected)
	require.Equal(t, expected, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return true, nil
	}))

	gateway = factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, expected
	}))
}

func TestGatewayShouldUpdate(t *testing.T) {
	t.Parallel()

	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway := factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	other := factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})

	// Have equal resource version
	gateway.gateway.ObjectMeta.ResourceVersion = `0`
	other.gateway.ObjectMeta.ResourceVersion = `0`
	assert.True(t, gateway.ShouldUpdate(other))

	// Have greater resource version
	gateway.gateway.ObjectMeta.ResourceVersion = `1`
	other.gateway.ObjectMeta.ResourceVersion = `0`
	assert.False(t, gateway.ShouldUpdate(other))

	// Have lesser resource version
	gateway.gateway.ObjectMeta.ResourceVersion = `0`
	other.gateway.ObjectMeta.ResourceVersion = `1`
	assert.True(t, gateway.ShouldUpdate(other))

	// Have non-numeric resource version
	gateway.gateway.ObjectMeta.ResourceVersion = `a`
	other.gateway.ObjectMeta.ResourceVersion = `0`
	assert.True(t, gateway.ShouldUpdate(other))

	// Other gateway non-numeric resource version
	gateway.gateway.ObjectMeta.ResourceVersion = `0`
	other.gateway.ObjectMeta.ResourceVersion = `a`
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
	gateway := factory.NewGateway(NewGatewayConfig{Gateway: &gw.Gateway{}})
	gateway.gateway.Name = "name"

	require.False(t, gateway.ShouldBind(storeMocks.NewMockRoute(nil)))

	route := factory.NewRoute(&gw.HTTPRoute{})
	route.resolutionErrors.Add(service.NewConsulResolutionError("test"))
	require.False(t, gateway.ShouldBind(route))

	require.True(t, gateway.ShouldBind(factory.NewRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "name",
				}},
			},
		},
	})))

	require.False(t, gateway.ShouldBind(factory.NewRoute(&gw.HTTPRoute{})))
}

func serviceType(v core.ServiceType) *core.ServiceType {
	return &v
}
