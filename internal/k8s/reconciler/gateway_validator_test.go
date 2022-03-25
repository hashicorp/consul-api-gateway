package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
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

	validator := NewGatewayValidator(client)

	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	_, err := validator.Validate(context.Background(), gateway)
	require.NoError(t, err)

	expected := errors.New("expected")
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	_, err = validator.Validate(context.Background(), gateway)
	require.True(t, errors.Is(err, expected))

	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected).Times(1)
	_, err = validator.Validate(context.Background(), gateway)
	require.True(t, errors.Is(err, expected))
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
			validator := NewGatewayValidator(client)
			state := &state.GatewayState{}
			assert.NoError(t, validator.validateGatewayIP(context.Background(), state, gateway.Gateway, gateway.serviceBuilder.Build()))

			require.Len(t, state.Addresses, 1)
			assert.Equal(t, tc.expectedIP, state.Addresses[0])

			assert.True(t, state.ServiceReady)
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
	validator := NewGatewayValidator(client)
	state, err := validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	require.Equal(t, status.ListenerConditionReasonProtocolConflict, state.Listeners[0].Status.Conflicted.Condition(0).Reason)
	require.Equal(t, status.ListenerConditionReasonProtocolConflict, state.Listeners[1].Status.Conflicted.Condition(0).Reason)
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
	validator := NewGatewayValidator(client)
	state, err := validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	require.Equal(t, status.ListenerConditionReasonHostnameConflict, state.Listeners[0].Status.Conflicted.Condition(0).Reason)
	require.Equal(t, status.ListenerConditionReasonHostnameConflict, state.Listeners[1].Status.Conflicted.Condition(0).Reason)
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
	validator := NewGatewayValidator(client)
	state, err := validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	require.Equal(t, status.GatewayConditionReasonUnknown, state.Status.Scheduled.Condition(0).Reason)

	// Pod has pending status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	require.Equal(t, status.GatewayConditionReasonNotReconciled, state.Status.Scheduled.Condition(0).Reason)

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
	state, err = validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonNoResources, state.Status.Scheduled.Condition(0).Reason)

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
	state, err = validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	assert.True(t, state.PodReady)

	// Pod has succeeded status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
		},
	}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonPodFailed, state.Status.Scheduled.Condition(0).Reason)

	// Pod has failed status
	client.EXPECT().PodWithLabels(gomock.Any(), gomock.Any()).Return(&core.Pod{
		Status: core.PodStatus{
			Phase: core.PodFailed,
		},
	}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonPodFailed, state.Status.Scheduled.Condition(0).Reason)
}

func TestListenerValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	validator := NewGatewayValidator(client)

	// protocols
	listener := NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	validator.validateProtocols(listener.ListenerState, listener.listener)
	condition := listener.ListenerState.Status.Detached.Condition(0)
	require.Equal(t, status.ListenerConditionReasonUnsupportedProtocol, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
		AllowedRoutes: &gw.AllowedRoutes{
			Kinds: []gw.RouteGroupKind{{
				Kind: gw.Kind("UDPRoute"),
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	validator.validateProtocols(listener.ListenerState, listener.listener)
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidRouteKinds, condition.Reason)

	// Addresses
	listener = NewK8sListener(&K8sGateway{
		Gateway: &gw.Gateway{
			Spec: gw.GatewaySpec{
				Addresses: []gw.GatewayAddress{{}},
			},
		},
	}, gw.Listener{
		Protocol: gw.HTTPProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	validator.validateUnsupported(listener.ListenerState, listener.gateway.Gateway)
	condition = listener.ListenerState.Status.Detached.Condition(0)
	require.Equal(t, status.ListenerConditionReasonUnsupportedAddress, condition.Reason)

	// TLS validations
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	err := validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)

	mode := gw.TLSModePassthrough
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			Mode: &mode,
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS:      &gw.GatewayTLSConfig{},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	})
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	expected := errors.New("expected")
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.True(t, errors.Is(err, expected))

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	require.Len(t, listener.Config().TLS.Certificates, 0)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	require.Len(t, listener.Config().TLS.Certificates, 1)

	group := gw.Group("group")
	kind := gw.Kind("kind")
	namespace := gw.Namespace("namespace")
	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Namespace: &namespace,
				Group:     &group,
				Kind:      &kind,
				Name:      "secret",
			}},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version": "TLSv1_2",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonReady, condition.Reason)
	require.Equal(t, "TLSv1_2", listener.ListenerState.TLS.MinVersion)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version": "foo",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "unrecognized TLS min version", condition.Message)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonReady, condition.Reason)
	require.Equal(t, []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, listener.ListenerState.TLS.CipherSuites)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version":   "TLSv1_3",
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "foo",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "configuring TLS cipher suites is only supported for TLS 1.2 and earlier", condition.Message)

	listener = NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Protocol: gw.HTTPSProtocolType,
		TLS: &gw.GatewayTLSConfig{
			CertificateRefs: []*gw.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gw.AnnotationKey]gw.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, foo",
			},
		},
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listener.ListenerState, listener.gateway.Gateway, listener.listener)
	require.NoError(t, err)
	condition = listener.ListenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "unrecognized or unsupported TLS cipher suite: foo", condition.Message)
}
