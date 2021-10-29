package reconciler

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	internalCore "github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	storeMocks "github.com/hashicorp/consul-api-gateway/internal/store/mocks"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestGatewayValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gw.Hostname("*")
	serviceType := core.ServiceTypeNodePort
	gateway := NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Hostname: &hostname,
				Protocol: gw.HTTPSProtocolType,
				TLS: &gw.GatewayTLSConfig{
					CertificateRefs: []*gw.SecretObjectReference{{}},
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
				ServiceType: &serviceType,
			},
		},
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, gateway.Validate(context.Background()))

	expected := errors.New("expected")
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.True(t, errors.Is(gateway.Validate(context.Background()), expected))

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.True(t, errors.Is(gateway.Validate(context.Background()), expected))
}

func TestGatewayValidate_ListenerProtocolConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gateway := NewK8sGateway(&gw.Gateway{
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
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, ListenerConditionReasonProtocolConflict, gateway.listeners["1"].status.Conflicted.Condition(0).Reason)
	require.Equal(t, ListenerConditionReasonProtocolConflict, gateway.listeners["2"].status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_ListenerHostnameConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gw.Hostname("1")
	other := gw.Hostname("2")
	gateway := NewK8sGateway(&gw.Gateway{
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
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, ListenerConditionReasonHostnameConflict, gateway.listeners["1"].status.Conflicted.Condition(0).Reason)
	require.Equal(t, ListenerConditionReasonHostnameConflict, gateway.listeners["2"].status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_Pods(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gateway := NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{},
	}, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonUnknown, gateway.status.Scheduled.Condition(0).Reason)

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonNotReconciled, gateway.status.Scheduled.Condition(0).Reason)

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodPending,
			Conditions: []core.PodCondition{{
				Type:   core.PodScheduled,
				Status: core.ConditionFalse,
				Reason: "Unschedulable",
			}},
		},
	}, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonNoResources, gateway.status.Scheduled.Condition(0).Reason)

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodRunning,
			Conditions: []core.PodCondition{{
				Type:   core.PodReady,
				Status: core.ConditionTrue,
			}},
		},
	}, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.True(t, gateway.podReady)

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
		},
	}, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonPodFailed, gateway.status.Scheduled.Condition(0).Reason)

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodFailed,
		},
	}, nil)
	require.NoError(t, gateway.Validate(context.Background()))
	require.Equal(t, GatewayConditionReasonPodFailed, gateway.status.Scheduled.Condition(0).Reason)
}

func TestGatewayID(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gw.Gateway{
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

	gateway := NewK8sGateway(&gw.Gateway{
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

	gateway := NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Len(t, gateway.Listeners(), 1)
}

func TestGatewayOutputStatus(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Name: gw.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.addresses = []string{"127.0.0.1"}
	gateway.listeners["1"].status.Ready.Pending = errors.New("pending")
	require.Len(t, gateway.Status().Addresses, 1)
	require.Equal(t, GatewayConditionReasonListenersNotReady, gateway.status.Ready.Condition(0).Reason)

	gateway = NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Name: gw.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.listeners["1"].status.Ready.Invalid = errors.New("invalid")
	require.Len(t, gateway.Status().Listeners, 1)
	require.Equal(t, GatewayConditionReasonListenersNotValid, gateway.status.Ready.Condition(0).Reason)

	gateway = NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Name: gw.SectionName("1"),
			}},
			Addresses: []gw.GatewayAddress{{}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.podReady = true
	require.Len(t, gateway.Status().Listeners, 1)
	require.Equal(t, GatewayConditionReasonAddressNotAssigned, gateway.status.Ready.Condition(0).Reason)

	gateway = NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Name: gw.SectionName("1"),
			}},
			Addresses: []gw.GatewayAddress{{}},
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

	gateway := NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	gateway.gateway.Status = gateway.Status()
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	expected := errors.New("expected")

	gateway = NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, expected)
	require.True(t, errors.Is(gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}), expected))

	gateway = NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(expected)
	require.Equal(t, expected, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, nil
	}))

	gateway = NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return true, nil
	}))

	gateway = NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: io.Discard,
			Level:  hclog.Trace,
		}),
		Client: client,
	})
	client.EXPECT().CreateOrUpdateDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	client.EXPECT().UpdateStatus(gomock.Any(), gateway.gateway).Return(nil)
	require.NoError(t, gateway.TrackSync(context.Background(), func() (bool, error) {
		return false, expected
	}))
}

func TestGatewayCompare(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	other := NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultEqual, gateway.Compare(other))
	require.Equal(t, store.CompareResultInvalid, gateway.Compare(nil))
	require.Equal(t, store.CompareResultInvalid, gateway.Compare(storeMocks.NewMockGateway(nil)))
	gateway = nil
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))

	gateway = NewK8sGateway(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			ResourceVersion: "1",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sGateway(&gw.Gateway{
		ObjectMeta: meta.ObjectMeta{
			ResourceVersion: "0",
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	require.Equal(t, store.CompareResultNewer, gateway.Compare(other))

	gateway.gateway.ObjectMeta.ResourceVersion = "0"
	gateway.gateway.Spec.GatewayClassName = "other"
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))

	gateway.gateway.Spec.GatewayClassName = ""
	gateway.gateway.Status.Conditions = []meta.Condition{{}}
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))

	gateway = NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Name: gw.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	other = NewK8sGateway(&gw.Gateway{
		Spec: gw.GatewaySpec{
			Listeners: []gw.Listener{{
				Name: gw.SectionName("1"),
			}},
		},
	}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.listeners["1"].certificates = []string{"other"}
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))

	gateway.listeners["1"].certificates = nil
	gateway.status.Scheduled.Unknown = errors.New("")
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))

	gateway.status.Scheduled.Unknown = nil
	gateway.podReady = true
	other.podReady = false
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))

	other.podReady = true
	gateway.addresses = []string{""}
	require.Equal(t, store.CompareResultNotEqual, gateway.Compare(other))
}

func TestGatewayShouldBind(t *testing.T) {
	t.Parallel()

	gateway := NewK8sGateway(&gw.Gateway{}, K8sGatewayConfig{
		Logger: hclog.NewNullLogger(),
	})
	gateway.gateway.Name = "name"

	require.False(t, gateway.ShouldBind(storeMocks.NewMockRoute(nil)))

	route := NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})
	route.resolutionErrors.Add(service.NewConsulResolutionError("test"))
	require.False(t, gateway.ShouldBind(route))

	require.True(t, gateway.ShouldBind(NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: gw.CommonRouteSpec{
				ParentRefs: []gw.ParentRef{{
					Name: "name",
				}},
			},
		},
	}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))

	require.False(t, gateway.ShouldBind(NewK8sRoute(&gw.HTTPRoute{}, K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	})))
}
