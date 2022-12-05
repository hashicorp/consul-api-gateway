// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package builder

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

var (
	generate bool
	fixtures = []string{
		"tls-cert",
		"static-mapping",
		"clusterip",
		"loadbalancer",
		"multiple-instances",
		"max-instances",
		"min-instances",
	}
)

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

type gatewayTestConfig struct {
	gatewayClassConfig *v1alpha1.GatewayClassConfig
	gatewayClass       *gwv1beta1.GatewayClass
	gateway            *gwv1beta1.Gateway
}

func newGatewayTestConfig() *gatewayTestConfig {
	return &gatewayTestConfig{
		gatewayClassConfig: &v1alpha1.GatewayClassConfig{},
		gatewayClass:       &gwv1beta1.GatewayClass{},
		gateway:            &gwv1beta1.Gateway{},
	}
}

func (g *gatewayTestConfig) EncodeDeployment() runtime.Object {
	return NewGatewayDeployment(g.gateway).
		WithSDS("consul-api-gateway-controller.default.svc.cluster.local", 9090).
		WithClassConfig(*g.gatewayClassConfig).
		WithConsulCA("CONSUL_CA_MOCKED").
		WithConsulGatewayNamespace("test").
		Build(nil)
}

func (g *gatewayTestConfig) EncodeService() runtime.Object {
	return NewGatewayService(g.gateway).
		WithClassConfig(*g.gatewayClassConfig).
		Build()
}

func TestGatewayDeploymentBuilder(t *testing.T) {
	t.Parallel()

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			config := newGatewayTestConfig()
			fixtureTest(t, name, "deployment", config, func() runtime.Object {
				return config.EncodeDeployment()
			})
		})
	}
}

func TestGatewayServiceBuilder(t *testing.T) {
	t.Parallel()

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			config := newGatewayTestConfig()
			fixtureTest(t, name, "service", config, func() runtime.Object {
				return config.EncodeService()
			})
		})
	}
}

func fixtureTest(t *testing.T, name, suffix string, into *gatewayTestConfig, encode func() runtime.Object) {
	t.Helper()

	file, err := os.OpenFile(path.Join("testdata", fmt.Sprintf("%s.yaml", name)), os.O_RDONLY, 0644)
	require.NoError(t, err)
	defer file.Close()

	stat, err := file.Stat()
	require.NoError(t, err)

	decoder := yaml.NewYAMLOrJSONDecoder(file, int(stat.Size()))
	err = decoder.Decode(into.gatewayClassConfig)
	require.NoError(t, err)
	err = decoder.Decode(into.gatewayClass)
	require.NoError(t, err)
	err = decoder.Decode(into.gateway)
	require.NoError(t, err)

	var buffer bytes.Buffer
	serializer := json.NewSerializerWithOptions(
		json.DefaultMetaFactory, nil, nil,
		json.SerializerOptions{
			Yaml:   true,
			Pretty: true,
			Strict: true,
		},
	)
	err = serializer.Encode(encode(), &buffer)
	require.NoError(t, err)

	var expected string
	expectedFileName := fmt.Sprintf("%s.%s.golden.yaml", name, suffix)
	if generate {
		expected = buffer.String()
		err := os.WriteFile(path.Join("testdata", expectedFileName), buffer.Bytes(), 0644)
		require.NoError(t, err)
	} else {
		data, err := os.ReadFile(path.Join("testdata", expectedFileName))
		require.NoError(t, err)
		expected = string(data)
	}

	require.Equal(t, expected, buffer.String())
}
