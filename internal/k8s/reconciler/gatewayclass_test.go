// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

func TestGatewayClassValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gatewayClass := NewK8sGatewayClass(&gwv1beta1.GatewayClass{}, K8sGatewayClassConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	require.NoError(t, gatewayClass.Validate(context.Background()))
	require.Equal(t, rstatus.GatewayClassConditionReasonAccepted, gatewayClass.status.Accepted.Condition(0).Reason)

	require.NoError(t, gatewayClass.Validate(context.Background()))
	gatewayClass = NewK8sGatewayClass(&gwv1beta1.GatewayClass{
		Spec: gwv1beta1.GatewayClassSpec{
			ParametersRef: &gwv1beta1.ParametersReference{
				Group: "group",
				Kind:  "kind",
			},
		},
	}, K8sGatewayClassConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	require.NoError(t, gatewayClass.Validate(context.Background()))
	require.Equal(t, rstatus.GatewayClassConditionReasonInvalidParameters, gatewayClass.status.Accepted.Condition(0).Reason)

	require.NoError(t, gatewayClass.Validate(context.Background()))
	gatewayClass = NewK8sGatewayClass(&gwv1beta1.GatewayClass{
		Spec: gwv1beta1.GatewayClassSpec{
			ParametersRef: &gwv1beta1.ParametersReference{
				Group: apigwv1alpha1.Group,
				Kind:  apigwv1alpha1.GatewayClassConfigKind,
				Name:  "config",
			},
		},
	}, K8sGatewayClassConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	expected := errors.New("expected")
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), gomock.Any()).Return(nil, expected)
	require.Equal(t, expected, gatewayClass.Validate(context.Background()))

	client.EXPECT().GetGatewayClassConfig(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, gatewayClass.Validate(context.Background()))
	require.Equal(t, rstatus.GatewayClassConditionReasonInvalidParameters, gatewayClass.status.Accepted.Condition(0).Reason)
	require.False(t, gatewayClass.IsValid())

	config := &apigwv1alpha1.GatewayClassConfig{}
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), gomock.Any()).Return(config, nil)
	require.NoError(t, gatewayClass.Validate(context.Background()))
	require.Equal(t, rstatus.GatewayClassConditionReasonAccepted, gatewayClass.status.Accepted.Condition(0).Reason)
	require.Equal(t, *config, gatewayClass.config)
	require.True(t, gatewayClass.IsValid())
}

func TestGatewayClasses(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	gatewayClass := NewK8sGatewayClass(&gwv1beta1.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			ResourceVersion: "1",
		},
		Spec: gwv1beta1.GatewayClassSpec{
			ParametersRef: &gwv1beta1.ParametersReference{
				Group: apigwv1alpha1.Group,
				Kind:  apigwv1alpha1.GatewayClassConfigKind,
				Name:  "config",
			},
		},
	}, K8sGatewayClassConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), gomock.Any()).Return(nil, nil)
	require.NoError(t, gatewayClass.Validate(context.Background()))

	classes := NewK8sGatewayClasses(hclog.NewNullLogger(), client)
	client.EXPECT().UpdateStatus(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, classes.Upsert(context.Background(), gatewayClass))
	_, found := classes.GetConfig("")
	require.False(t, found)

	config := &apigwv1alpha1.GatewayClassConfig{}
	client.EXPECT().GetGatewayClassConfig(gomock.Any(), gomock.Any()).Return(config, nil)
	require.NoError(t, gatewayClass.Validate(context.Background()))
	require.True(t, gatewayClass.IsValid())

	classes = NewK8sGatewayClasses(hclog.NewNullLogger(), client)
	client.EXPECT().UpdateStatus(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, classes.Upsert(context.Background(), gatewayClass))
	_, found = classes.GetConfig("")
	require.True(t, found)

	gatewayClass = NewK8sGatewayClass(&gwv1beta1.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			ResourceVersion: "0",
		},
		Spec: gwv1beta1.GatewayClassSpec{
			ParametersRef: &gwv1beta1.ParametersReference{
				Group: apigwv1alpha1.Group,
				Kind:  apigwv1alpha1.GatewayClassConfigKind,
				Name:  "config",
			},
		},
	}, K8sGatewayClassConfig{
		Logger: hclog.NewNullLogger(),
		Client: client,
	})
	require.NoError(t, classes.Upsert(context.Background(), gatewayClass))
}
