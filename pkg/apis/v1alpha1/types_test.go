package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGatewayClassConfigDeepCopy(t *testing.T) {
	var nilConfig *GatewayClassConfig
	require.Nil(t, nilConfig.DeepCopy())
	require.Nil(t, nilConfig.DeepCopyObject())
	lbType := core.ServiceTypeLoadBalancer
	spec := GatewayClassConfigSpec{
		ServiceType: &lbType,
		NodeSelector: map[string]string{
			"test": "test",
		},
	}
	config := &GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: "test",
		},
		Spec: spec,
	}
	copy := config.DeepCopy()
	copyObject := config.DeepCopyObject()
	require.Equal(t, copy, copyObject)

	var nilSpec *GatewayClassConfigSpec
	require.Nil(t, nilSpec.DeepCopy())
	specCopy := (&spec).DeepCopy()
	require.Equal(t, spec.NodeSelector, specCopy.NodeSelector)

	var nilConfigList *GatewayClassConfigList
	require.Nil(t, nilConfigList.DeepCopyObject())
	configList := &GatewayClassConfigList{
		Items: []GatewayClassConfig{*config},
	}
	copyConfigList := configList.DeepCopy()
	copyConfigListObject := configList.DeepCopyObject()
	require.Equal(t, copyConfigList, copyConfigListObject)
}
