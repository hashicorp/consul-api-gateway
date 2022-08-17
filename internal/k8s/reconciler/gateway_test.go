package reconciler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGatewayValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gwv1beta1.Hostname("*")
	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Hostname: &hostname,
				Protocol: gwv1beta1.HTTPSProtocolType,
				TLS: &gwv1beta1.GatewayTLSConfig{
					CertificateRefs: []gwv1beta1.SecretObjectReference{{}},
				},
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{

				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))

	expected := errors.New("expected")
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.True(t, errors.Is(gateway.Validate(context.Background()), expected))

	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected).Times(1)
	require.True(t, errors.Is(gateway.Validate(context.Background()), expected))
}

func TestGatewayValidateGatewayIP(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gwv1beta1.Hostname("*")

	gwDef := &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Hostname: &hostname,
				Protocol: gwv1beta1.HTTPSProtocolType,
				TLS: &gwv1beta1.GatewayTLSConfig{
					CertificateRefs: []gwv1beta1.SecretObjectReference{{}},
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
					{
						Hostname: "this.is.a.hostname",
					},
				},
			},
		},
	}

	for _, tc := range []struct {
		// What IP address do we expect the Gateway to be assigned?
		expectedIPs []string

		// Should the mock client expect a request for the Service?
		// If false, the mock client expects a request for the Pod instead.
		expectedIPFromSvc bool

		// What serviceType should the gateway be configured for?
		serviceType *core.ServiceType
	}{
		{
			expectedIPs:       []string{pod.Status.PodIP},
			expectedIPFromSvc: false,
			serviceType:       nil,
		},
		{
			expectedIPs:       []string{pod.Status.HostIP},
			expectedIPFromSvc: false,
			serviceType:       serviceType(core.ServiceTypeNodePort),
		},
		{
			expectedIPs:       []string{svc.Status.LoadBalancer.Ingress[0].IP, svc.Status.LoadBalancer.Ingress[1].Hostname},
			expectedIPFromSvc: true,
			serviceType:       serviceType(core.ServiceTypeLoadBalancer),
		},
		{
			expectedIPs:       []string{svc.Spec.ClusterIP},
			expectedIPFromSvc: true,
			serviceType:       serviceType(core.ServiceTypeClusterIP),
		},
	} {
		name := "Service type <nil>"
		if tc.serviceType != nil {
			name = fmt.Sprintf("Service type %s", *tc.serviceType)
		}

		t.Run(name, func(t *testing.T) {
			gateway := NewK8sGateway(gwDef, K8sGatewayConfig{
				Logger: hclog.NewNullLogger(),
				Client: client,
				Config: apigwv1alpha1.GatewayClassConfig{
					Spec: apigwv1alpha1.GatewayClassConfigSpec{
						ServiceType: tc.serviceType,
					},
				},
			})

			if tc.expectedIPFromSvc {
				client.EXPECT().GetService(gomock.Any(), gomock.Any()).Return(svc, nil)
			} else {
				client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{*pod}, nil)
			}
			assert.NoError(t, gateway.validateGatewayIP(context.Background()))

			require.Len(t, gateway.addresses, len(tc.expectedIPs))
			assert.Equal(t, tc.expectedIPs, gateway.addresses)

			assert.True(t, gateway.serviceReady)
		})
	}
}

func TestGatewayValidate_ListenerProtocolConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name:     gwv1beta1.SectionName("1"),
				Protocol: gwv1beta1.HTTPProtocolType,
				Port:     gwv1beta1.PortNumber(1),
			}, {
				Name:     gwv1beta1.SectionName("2"),
				Protocol: gwv1beta1.UDPProtocolType,
				Port:     gwv1beta1.PortNumber(1),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
		Client: client,
	})
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, rstatus.ListenerConditionReasonProtocolConflict, gateway.listeners["1"].status.Conflicted.Condition(0).Reason)
	require.Equal(t, rstatus.ListenerConditionReasonProtocolConflict, gateway.listeners["2"].status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_ListenerHostnameConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gwv1beta1.Hostname("1")
	other := gwv1beta1.Hostname("2")
	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name:     gwv1beta1.SectionName("1"),
				Protocol: gwv1beta1.HTTPProtocolType,
				Hostname: &hostname,
				Port:     gwv1beta1.PortNumber(1),
			}, {
				Name:     gwv1beta1.SectionName("2"),
				Protocol: gwv1beta1.HTTPProtocolType,
				Hostname: &other,
				Port:     gwv1beta1.PortNumber(1),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
		Client: client,
	})
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, rstatus.ListenerConditionReasonHostnameConflict, gateway.listeners["1"].status.Conflicted.Condition(0).Reason)
	require.Equal(t, rstatus.ListenerConditionReasonHostnameConflict, gateway.listeners["2"].status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_Pods(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				ServiceType: serviceType(core.ServiceTypeNodePort),
			},
		},
		Client: client,
	})

	// Pod has no/unknown status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{},
	}}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, rstatus.GatewayConditionReasonUnknown, gateway.status.Scheduled.Condition(0).Reason)

	// Pod has pending status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, rstatus.GatewayConditionReasonNotReconciled, gateway.status.Scheduled.Condition(0).Reason)

	// Pod is marked as unschedulable
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{
		{
			Status: core.PodStatus{
				Phase: core.PodPending,
				Conditions: []core.PodCondition{{
					Type:   core.PodScheduled,
					Status: core.ConditionFalse,
					Reason: "Unschedulable",
				}},
			},
		}}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.Equal(t, rstatus.GatewayConditionReasonNoResources, gateway.status.Scheduled.Condition(0).Reason)

	// Pod has running status and is marked ready
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodRunning,
			Conditions: []core.PodCondition{{
				Type:   core.PodReady,
				Status: core.ConditionTrue,
			}},
		},
	}}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.True(t, gateway.podReady)

	// Pod has succeeded status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
		},
	}}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.Equal(t, rstatus.GatewayConditionReasonPodFailed, gateway.status.Scheduled.Condition(0).Reason)

	// Pod has failed status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodFailed,
		},
	}}, nil).Times(2)
	require.NoError(t, gateway.Validate(context.Background()))
	assert.Equal(t, rstatus.GatewayConditionReasonPodFailed, gateway.status.Scheduled.Condition(0).Reason)
}

func TestGatewayID(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}, K8sGatewayConfig{
		Logger:          hclog.NewNullLogger(),
		ConsulNamespace: "consul",
	})
	require.Equal(t, internalCore.GatewayID{Service: "name", ConsulNamespace: "consul"}, gateway.ID())
}

func TestGatewayMeta(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}, K8sGatewayConfig{
		Logger:          hclog.NewNullLogger(),
		ConsulNamespace: "consul",
	})
	require.NotNil(t, gateway.Meta())
}

func TestGatewayListeners(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Len(t, gateway.Listeners(), 1)
}

func TestGatewayOutputStatus(t *testing.T) {
	t.Parallel()

	// Pending listener
	gateway := NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.addresses = []string{"127.0.0.1"}
	gateway.listeners["1"].status.Ready.Pending = errors.New("pending")
	require.Len(t, gateway.Status().Addresses, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonListenersNotReady, gateway.status.Ready.Condition(0).Reason)

	// Service ready, pods not
	gateway = NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.podReady = false
	gateway.serviceReady = true
	gateway.listeners["1"].status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonListenersNotValid, gateway.status.Ready.Condition(0).Reason)

	// Pods ready, service not
	gateway = NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.podReady = true
	gateway.serviceReady = false
	gateway.listeners["1"].status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonListenersNotValid, gateway.status.Ready.Condition(0).Reason)

	// Pods + service ready
	gateway = NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
			Addresses: []gwv1beta1.GatewayAddress{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.podReady = true
	gateway.serviceReady = true
	require.Len(t, gateway.Status().Listeners, 1)
	assert.Equal(t, rstatus.GatewayConditionReasonAddressNotAssigned, gateway.status.Ready.Condition(0).Reason)

	gateway = NewK8sGateway(&gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{
				Name: gwv1beta1.SectionName("1"),
			}},
			Addresses: []gwv1beta1.GatewayAddress{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.gateway.Status = gateway.Status()
	gateway.Status()
}

func TestGatewayTrackSync(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gateway := NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	gateway.gateway.Status = gateway.Status()
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	var instances int32 = 2
	gateway = NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
		Config: apigwv1alpha1.GatewayClassConfig{
			Spec: apigwv1alpha1.GatewayClassConfigSpec{
				DeploymentSpec: apigwv1alpha1.DeploymentSpec{
					DefaultInstances: &instances,
				},
			},
		},
	})

	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	expected := errors.New("expected")

	gateway = NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	require.True(t, errors.Is(gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}), expected))

	gateway = NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(expected)
	require.Equal(t, expected, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})

	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return true, nil
	}))

	gateway = NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, expected
	}))
}

func TestGatewayShouldUpdate(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})

	other := NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})

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

	gateway := NewK8sGateway(&gwv1beta1.Gateway{}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.gateway.Name = "name"

	require.False(t, gateway.ShouldBind(storeMocks.NewMockRoute(nil)))

	route := NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	route.resolutionErrors.Add(service.NewConsulResolutionError("test"))
	require.False(t, gateway.ShouldBind(route))

	require.True(t, gateway.ShouldBind(NewK8sRoute(&gwv1alpha2.HTTPRoute{
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

	require.False(t, gateway.ShouldBind(NewK8sRoute(&gwv1alpha2.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))
}

func serviceType(v core.ServiceType) *core.ServiceType {
	return &v
}
