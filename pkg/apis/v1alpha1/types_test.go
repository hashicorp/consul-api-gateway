package v1alpha1

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	generate bool
	fixtures = []string{
		"tls-cert",
		"static-mapping",
		"clusterip",
		"loadbalancer",
	}
)

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

type gatewayTestConfig struct {
	gatewayClassConfig *GatewayClassConfig
	gatewayClass       *gateway.GatewayClass
	gateway            *gateway.Gateway
}

func newGatewayTestConfig() *gatewayTestConfig {
	return &gatewayTestConfig{
		gatewayClassConfig: &GatewayClassConfig{},
		gatewayClass:       &gateway.GatewayClass{},
		gateway:            &gateway.Gateway{},
	}
}

func (g *gatewayTestConfig) EncodeDeployment() runtime.Object {
	return g.gatewayClassConfig.DeploymentFor(g.gateway, SDSConfig{
		Host: "consul-api-gateway-controller.default.svc.cluster.local",
		Port: 9090,
	})
}

func (g *gatewayTestConfig) EncodeService() runtime.Object {
	return g.gatewayClassConfig.ServiceFor(g.gateway)
}

func TestDeploymentFor(t *testing.T) {
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

func TestServiceFor(t *testing.T) {
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
