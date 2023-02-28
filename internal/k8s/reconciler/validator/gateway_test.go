// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
	state, err := validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.NotNil(t, state)

	expected := errors.New("expected")
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
	state, err = validator.Validate(context.Background(), gateway, nil)
	require.True(t, errors.Is(err, expected))
	assert.Nil(t, state)

	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return(nil, expected).Times(1)
	state, err = validator.Validate(context.Background(), gateway, nil)
	require.True(t, errors.Is(err, expected))
	assert.Nil(t, state)
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
	gwState, err := validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	require.Equal(t, status.GatewayConditionReasonUnknown, gwState.Status.Scheduled.Condition(0).Reason)

	// Pod has pending status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}}, nil).Times(2)
	gwState, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	require.Equal(t, status.GatewayConditionReasonNotReconciled, gwState.Status.Scheduled.Condition(0).Reason)

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
	gwState, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonNoResources, gwState.Status.Scheduled.Condition(0).Reason)

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
	gwState, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.True(t, gwState.PodReady)

	// Pod has succeeded status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
		},
	}}, nil).Times(2)
	gwState, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonPodFailed, gwState.Status.Scheduled.Condition(0).Reason)

	// Pod has failed status
	client.EXPECT().PodsWithLabels(gomock.Any(), gomock.Any()).Return([]core.Pod{{
		Status: core.PodStatus{
			Phase: core.PodFailed,
		},
	}}, nil).Times(2)
	gwState, err = validator.Validate(context.Background(), gateway, nil)
	require.NoError(t, err)
	assert.Equal(t, status.GatewayConditionReasonPodFailed, gwState.Status.Scheduled.Condition(0).Reason)
}

func TestListenerValidate(t *testing.T) {
	t.Parallel()

	expected := errors.New("expected")
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	validator := NewGatewayValidator(client)

	t.Run("Unsupported protocol", func(t *testing.T) {
		listener := gwv1beta1.Listener{}
		listenerState := &state.ListenerState{}
		validator.validateProtocols(listenerState, listener)
		condition := listenerState.Status.Detached.Condition(0)
		require.Equal(t, status.ListenerConditionReasonUnsupportedProtocol, condition.Reason)
	})

	t.Run("Invalid route kinds", func(t *testing.T) {
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPProtocolType,
			AllowedRoutes: &gwv1beta1.AllowedRoutes{
				Kinds: []gwv1beta1.RouteGroupKind{{
					Kind: gwv1beta1.Kind("UDPRoute"),
				}},
			},
		}
		listenerState := &state.ListenerState{}
		validator.validateProtocols(listenerState, listener)
		condition := listenerState.Status.ResolvedRefs.Condition(0)
		require.Equal(t, status.ListenerConditionReasonInvalidRouteKinds, condition.Reason)
	})

	t.Run("Unsupported address", func(t *testing.T) {
		gateway := &gwv1beta1.Gateway{
			Spec: gwv1beta1.GatewaySpec{
				Addresses: []gwv1beta1.GatewayAddress{{}},
			},
		}
		listenerState := &state.ListenerState{}
		validator.validateUnsupported(listenerState, gateway)
		condition := listenerState.Status.Detached.Condition(0)
		require.Equal(t, status.ListenerConditionReasonUnsupportedAddress, condition.Reason)
	})

	t.Run("Invalid TLS config", func(t *testing.T) {
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
		}
		listenerState := &state.ListenerState{}
		validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		condition := listenerState.Status.Ready.Condition(0)
		require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	})

	t.Run("Invalid TLS passthrough", func(t *testing.T) {
		mode := gwv1beta1.TLSModePassthrough
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				Mode: &mode,
			},
		}
		listenerState := &state.ListenerState{}
		validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		condition := listenerState.Status.Ready.Condition(0)
		require.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
	})

	t.Run("Invalid certificate ref", func(t *testing.T) {
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS:      &gwv1beta1.GatewayTLSConfig{},
		}
		listenerState := &state.ListenerState{}
		validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		condition := listenerState.Status.ResolvedRefs.Condition(0)
		require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Fail to retrieve secret", func(t *testing.T) {
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
			},
		}
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, expected)
		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.True(t, errors.Is(err, expected))
	})

	t.Run("No secret found", func(t *testing.T) {
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
			},
		}
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(nil, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.ResolvedRefs.Condition(0)
		require.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Invalid cross-namespace secret ref with no ReferenceGrant", func(t *testing.T) {
		otherNamespace := gwv1beta1.Namespace("other-namespace")
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Namespace: &otherNamespace,
					Name:      "secret",
				}},
			},
		}
		listenerState := &state.ListenerState{}
		client.EXPECT().GetReferenceGrantsInNamespace(gomock.Any(), string(otherNamespace)).Return([]gwv1alpha2.ReferenceGrant{}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.ResolvedRefs.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonRefNotPermitted, condition.Reason)
	})

	t.Run("Valid cross-namespace secret ref with ReferenceGrant", func(t *testing.T) {
		gatewayNamespace := gwv1alpha2.Namespace("gateway-namespace")
		secretNamespace := gwv1beta1.Namespace("secret-namespace")
		gateway := &gwv1beta1.Gateway{
			ObjectMeta: meta.ObjectMeta{Namespace: string(gatewayNamespace)},
			TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1beta1", Kind: "Gateway"},
		}
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Namespace: &secretNamespace,
					Name:      "secret",
				}},
			},
		}
		listenerState := &state.ListenerState{}
		client.EXPECT().GetReferenceGrantsInNamespace(gomock.Any(), string(secretNamespace)).
			Return([]gwv1alpha2.ReferenceGrant{{
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Namespace: gatewayNamespace,
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Kind: "Secret",
					}},
				},
			}}, nil)
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, gateway, listener)
		require.NoError(t, err)
		assert.Len(t, listenerState.TLS.Certificates, 1)

		condition := listenerState.Status.ResolvedRefs.Condition(0)
		assert.Equal(t, meta.ConditionTrue, condition.Status)
	})

	t.Run("Valid same-namespace secret ref without ReferenceGrant", func(t *testing.T) {
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Name: "secret",
				}},
			},
		}
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)
		assert.Len(t, listenerState.TLS.Certificates, 1)
	})

	t.Run("Unsupported certificate type", func(t *testing.T) {
		group := gwv1beta1.Group("group")
		kind := gwv1beta1.Kind("kind")
		listener := gwv1beta1.Listener{
			Protocol: gwv1beta1.HTTPSProtocolType,
			TLS: &gwv1beta1.GatewayTLSConfig{
				CertificateRefs: []gwv1beta1.SecretObjectReference{{
					Group: &group,
					Kind:  &kind,
					Name:  "secret",
				}},
			},
		}
		listenerState := &state.ListenerState{}

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.ResolvedRefs.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonInvalidCertificateRef, condition.Reason)
	})

	t.Run("Valid minimum TLS version", func(t *testing.T) {
		listener := gwv1beta1.Listener{
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
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.Ready.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonReady, condition.Reason)
		assert.Equal(t, "TLSv1_2", listenerState.TLS.MinVersion)
	})

	t.Run("Invalid minimum TLS version", func(t *testing.T) {
		listener := gwv1beta1.Listener{
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
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.Ready.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
		assert.Equal(t, "unrecognized TLS min version", condition.Message)
	})

	t.Run("Valid TLS cipher suite", func(t *testing.T) {
		listener := gwv1beta1.Listener{
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
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.Ready.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonReady, condition.Reason)
		assert.Equal(t, []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, listenerState.TLS.CipherSuites)
	})

	t.Run("TLS cipher suite not allowed", func(t *testing.T) {
		listener := gwv1beta1.Listener{
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
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.Ready.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
		assert.Equal(t, "configuring TLS cipher suites is only supported for TLS 1.2 and earlier", condition.Message)
	})

	t.Run("Invalid TLS cipher suite", func(t *testing.T) {
		listener := gwv1beta1.Listener{
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
		listenerState := &state.ListenerState{}
		client.EXPECT().GetSecret(gomock.Any(), gomock.Any()).Return(&core.Secret{
			ObjectMeta: meta.ObjectMeta{
				Name: "secret",
			},
		}, nil)

		err := validator.validateTLS(context.Background(), listenerState, &gwv1beta1.Gateway{}, listener)
		require.NoError(t, err)

		condition := listenerState.Status.Ready.Condition(0)
		assert.Equal(t, status.ListenerConditionReasonInvalid, condition.Reason)
		assert.Equal(t, "unrecognized or unsupported TLS cipher suite: foo", condition.Message)
	})
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

func TestGatewayAllowedForSecretRef(t *testing.T) {
	type testCase struct {
		name        string
		fromNS      string
		toNS        *string
		toKind      *string
		toName      string
		grantFromNS string
		grantToName *string
		allowed     bool
	}

	ns1, ns2, ns3 := "namespace1", "namespace2", "namespace3"
	secret1, secret2, secret3 := "secret1", "secret2", "secret3"

	for _, tc := range []testCase{
		{name: "unspecified-secret-namespace-allowed", fromNS: ns1, toNS: nil, toName: secret1, grantToName: nil, allowed: true},
		{name: "same-namespace-no-name-allowed", fromNS: ns1, toNS: &ns1, toName: secret1, grantToName: nil, allowed: true},
		{name: "same-namespace-with-name-allowed", fromNS: ns1, toNS: &ns1, toName: secret1, grantToName: &secret1, allowed: true},
		{name: "different-namespace-no-name-allowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "different-namespace-with-name-allowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns1, grantToName: &secret2, allowed: true},
		{name: "mismatched-grant-from-namespace-disallowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns3, grantToName: &secret2, allowed: false},
		{name: "mismatched-grant-to-name-disallowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns1, grantToName: &secret3, allowed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mocks.NewMockClient(ctrl)

			group := gwv1beta1.Group("")

			secretRef := gwv1beta1.SecretObjectReference{
				Group: &group,
				Name:  gwv1beta1.ObjectName(tc.toName),
			}

			if tc.toNS != nil {
				ns := gwv1beta1.Namespace(*tc.toNS)
				secretRef.Namespace = &ns
			}

			if tc.toKind != nil {
				k := gwv1beta1.Kind(*tc.toKind)
				secretRef.Kind = &k
			}

			gateway := &gwv1beta1.Gateway{
				TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "Gateway"},
				ObjectMeta: meta.ObjectMeta{Namespace: tc.fromNS},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						TLS: &gwv1beta1.GatewayTLSConfig{
							CertificateRefs: []gwv1beta1.SecretObjectReference{{
								Group: &group,
								Name:  gwv1beta1.ObjectName(tc.toName),
							}},
						},
					}},
				},
			}

			var toName *gwv1alpha2.ObjectName
			if tc.grantToName != nil {
				on := gwv1alpha2.ObjectName(*tc.grantToName)
				toName = &on
			}

			if tc.toNS != nil && tc.fromNS != *tc.toNS {
				otherName := gwv1alpha2.ObjectName("blah")

				refGrants := []gwv1alpha2.ReferenceGrant{
					// Create a ReferenceGrant that does not match at all (kind, etc.)
					{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{Group: "Kool & The Gang", Kind: "Jungle Boogie", Namespace: "Wild And Peaceful"}},
							To:   []gwv1alpha2.ReferenceGrantTo{{Group: "does not exist", Kind: "does not exist", Name: nil}},
						},
					},
					// Create a ReferenceGrant that matches completely except for To.Name
					{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{Group: "gateway.networking.k8s.io", Kind: gwv1alpha2.Kind("Gateway"), Namespace: gwv1alpha2.Namespace(tc.grantFromNS)}},
							To:   []gwv1alpha2.ReferenceGrantTo{{Group: "", Kind: "Secret", Name: &otherName}},
						},
					},
					// Create a ReferenceGrant that matches completely
					{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{Group: "gateway.networking.k8s.io", Kind: gwv1alpha2.Kind("Gateway"), Namespace: gwv1alpha2.Namespace(tc.grantFromNS)}},
							To:   []gwv1alpha2.ReferenceGrantTo{{Group: "", Kind: "Secret", Name: toName}},
						},
					},
				}

				client.EXPECT().
					GetReferenceGrantsInNamespace(gomock.Any(), *tc.toNS).
					Return(refGrants, nil)
			}

			allowed, err := gatewayAllowedForSecretRef(context.Background(), gateway, secretRef, client)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowed)
		})
	}
}
