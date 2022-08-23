package validator

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
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/builder"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGatewayValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gwv1beta1.Hostname("*")
	gateway := &gwv1beta1.Gateway{
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

	validator := NewGatewayValidator(client)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	_, err := validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)

	expected := errors.New("expected")
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	_, err = validator.Validate(context.Background(), gateway, nil)
	require.True(t, errors.Is(err, expected))

	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected).Times(1)
	_, err = validator.Validate(context.Background(), gateway, nil)
	require.True(t, errors.Is(err, expected))
}

func TestGatewayValidateGatewayIP(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gwv1beta1.Hostname("*")

	gateway := &gwv1beta1.Gateway{
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

	pod := core.Pod{
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
			config := apigwv1alpha1.GatewayClassConfig{
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ServiceType: tc.serviceType,
				},
			}

			if tc.expectedIPFromSvc {
				client.EXPECT().GetService(gomock.Any(), gomock.Any()).Return(svc, nil)
			} else {
				client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{pod}, nil)
			}
			validator := NewGatewayValidator(client)
			state := &state.GatewayState{}
			service := serviceFor(config, gateway)
			assert.NoError(t, validator.validateGatewayIP(context.Background(), state, gateway, service))

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

	gateway := &gwv1beta1.Gateway{
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
	}

	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	validator := NewGatewayValidator(client)
	state, err := validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	require.Equal(t, status.ListenerConditionReasonProtocolConflict, state.Listeners[0].Status.Conflicted.Condition(0).Reason)
	require.Equal(t, status.ListenerConditionReasonProtocolConflict, state.Listeners[1].Status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_ListenerHostnameConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	hostname := gwv1beta1.Hostname("1")
	other := gwv1beta1.Hostname("2")
	gateway := &gwv1beta1.Gateway{
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
	}

	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	validator := NewGatewayValidator(client)
	state, err := validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	require.Equal(t, status.ListenerConditionReasonHostnameConflict, state.Listeners[0].Status.Conflicted.Condition(0).Reason)
	require.Equal(t, status.ListenerConditionReasonHostnameConflict, state.Listeners[1].Status.Conflicted.Condition(0).Reason)
}

func TestGatewayValidate_Pods(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gateway := &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Listeners: []gwv1beta1.Listener{{}},
		},
	}

	// Pod has no/unknown status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{},
	}}, nil).Times(2)
	validator := NewGatewayValidator(client)
	state, err := validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	require.Equal(t, status.GatewayConditionReasonUnknown, state.Status.Scheduled.Condition(0).Reason)

	// Pod has pending status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	require.Equal(t, status.GatewayConditionReasonNotReconciled, state.Status.Scheduled.Condition(0).Reason)

	// Pod is marked as unschedulable
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodPending,
			Conditions: []core.PodCondition{{
				Type:   core.PodScheduled,
				Status: core.ConditionFalse,
				Reason: "Unschedulable",
			}},
		},
	}}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonNoResources, state.Status.Scheduled.Condition(0).Reason)

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
	state, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.True(t, state.PodReady)

	// Pod has succeeded status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
		},
	}}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonPodFailed, state.Status.Scheduled.Condition(0).Reason)

	// Pod has failed status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodFailed,
		},
	}}, nil).Times(2)
	state, err = validator.Validate(context.Background(), gateway, nil)
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
	listener := gwv1beta1.Listener{}
	listenerState := &state.ListenerState{}
	validator.validateProtocols(listenerState, listener)
	condition := listenerState.Status.Detached.Condition(0)
	require.Equal(t, status.ListenerConditionReasonUnsupportedProtocol, condition.Reason)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPProtocolType,
		AllowedRoutes: &gwv1beta1.AllowedRoutes{
			Kinds: []gwv1beta1.RouteGroupKind{{
				Kind: "UDPRoute",
			}},
		},
	}
	listenerState = &state.ListenerState{}
	validator.validateProtocols(listenerState, listener)
	condition = listenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidRouteKinds, condition.Reason)

	// Addresses
	gateway := &gwv1beta1.Gateway{
		Spec: gwv1beta1.GatewaySpec{
			Addresses: []gwv1beta1.GatewayAddress{{}},
		},
	}
	listenerState = &state.ListenerState{}
	validator.validateUnsupported(listenerState, gateway)
	condition = listenerState.Status.Detached.Condition(0)
	require.Equal(t, status.ListenerConditionReasonUnsupportedAddress, condition.Reason)

	// TLS validations
	gateway = &gwv1beta1.Gateway{}
	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
	}
	listenerState = &state.ListenerState{}
	err := validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)

	mode := gwv1beta1.TLSModePassthrough
	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			Mode: &mode,
		},
	}
	listenerState = &state.ListenerState{}
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS:      &gwv1beta1.GatewayTLSConfig{},
	}
	listenerState = &state.ListenerState{}
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	expected := errors.New("expected")
	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}
	listenerState = &state.ListenerState{}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.True(t, errors.Is(err, expected))

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}
	listenerState = &state.ListenerState{}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
		},
	}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	listenerState = &state.ListenerState{}
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	require.Len(t, listenerState.TLS.Certificates, 1)

	group := gwv1beta1.Group("group")
	kind := gwv1beta1.Kind("kind")
	namespace := gwv1beta1.Namespace("namespace")
	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Namespace: &namespace,
				Group:     &group,
				Kind:      &kind,
				Name:      "secret",
			}},
		},
	}
	listenerState = &state.ListenerState{}
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.ResolvedRefs.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version": "TLSv1_2",
			},
		},
	}
	listenerState = &state.ListenerState{}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonReady, condition.Reason)
	require.Equal(t, "TLSv1_2", listenerState.TLS.MinVersion)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version": "foo",
			},
		},
	}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	listenerState = &state.ListenerState{}
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "unrecognized TLS min version", condition.Message)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	listenerState = &state.ListenerState{}
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonReady, condition.Reason)
	require.Equal(t, []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, listenerState.TLS.CipherSuites)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_min_version":   "TLSv1_3",
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "foo",
			},
		},
	}
	listenerState = &state.ListenerState{}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "configuring TLS cipher suites is only supported for TLS 1.2 and earlier", condition.Message)

	listener = gwv1beta1.Listener{
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name: "secret",
			}},
			Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
				"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, foo",
			},
		},
	}
	listenerState = &state.ListenerState{}
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: "secret",
		},
	}, nil)
	err = validator.validateTLS(context.Background(), listenerState, gateway, listener)
	require.NoError(t, err)
	condition = listenerState.Status.Ready.Condition(0)
	require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	require.Equal(t, "unrecognized or unsupported TLS cipher suite: foo", condition.Message)
}

func TestIsKindInSet(t *testing.T) {
	t.Parallel()

	require.False(t, isKindInSet(gwv1beta1.RouteGroupKind{
		Kind: "test",
	}, []gwv1beta1.RouteGroupKind{}))
	require.True(t, isKindInSet(gwv1beta1.RouteGroupKind{
		Kind: "test",
	}, []gwv1beta1.RouteGroupKind{{
		Kind: "test",
	}}))

	group := gwv1beta1.Group("group")
	require.True(t, isKindInSet(gwv1beta1.RouteGroupKind{
		Kind:  "test",
		Group: &group,
	}, []gwv1beta1.RouteGroupKind{{
		Kind:  "test",
		Group: &group,
	}}))
}

func serviceType(v core.ServiceType) *core.ServiceType {
	return &v
}

func serviceFor(config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) *core.Service {
	serviceBuilder := builder.NewGatewayService(gateway)
	serviceBuilder.WithClassConfig(config)
	return serviceBuilder.Build()
}
