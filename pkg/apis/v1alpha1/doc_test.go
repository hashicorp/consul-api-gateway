package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func TestRegisterTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	RegisterTypes(scheme)

	var foundGatewayClassConfig bool
	var foundGatewayClassConfigList bool
	for gvk := range scheme.AllKnownTypes() {
		if gvk.GroupVersion() == GroupVersion {
			if gvk.Kind == "GatewayClassConfig" {
				foundGatewayClassConfig = true
			}
			if gvk.Kind == "GatewayClassConfigList" {
				foundGatewayClassConfigList = true
			}
		}
	}
	require.True(t, foundGatewayClassConfig)
	require.True(t, foundGatewayClassConfigList)
}
