package controllers

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	generate bool
	fixtures = []string{
		"basic",
		"annotations",
		"tls-cert",
		"node-selector",
		"invalid-node-selector",
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

func TestDeploymentFor(t *testing.T) {
	t.Parallel()

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			gw := &gateway.Gateway{}
			fixtureTest(t, name, "deployment", gw, func() runtime.Object {
				return DeploymentFor(gw)
			})
		})
	}
}

func TestServiceFor(t *testing.T) {
	t.Parallel()

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			gw := &gateway.Gateway{}
			fixtureTest(t, name, "service", gw, func() runtime.Object {
				return ServiceFor(gw)
			})
		})
	}
}

func TestNamespacedCASecretFor(t *testing.T) {
	t.Parallel()

	secret := namespacedCASecretFor(&gateway.Gateway{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "foo",
			Annotations: map[string]string{
				annotationConsulCASecret: "bar",
			},
		},
	})
	require.Equal(t, "foo/bar", secret.String())

	secret = namespacedCASecretFor(&gateway.Gateway{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "default",
		},
	})
	require.Equal(t, "default/consul-ca-cert", secret.String())
}

func fixtureTest(t *testing.T, name, suffix string, into interface{}, encode func() runtime.Object) {
	t.Helper()

	file, err := os.OpenFile(path.Join("testdata", fmt.Sprintf("%s.yaml", name)), os.O_RDONLY, 0644)
	require.NoError(t, err)
	defer file.Close()

	stat, err := file.Stat()
	require.NoError(t, err)

	err = yaml.NewYAMLOrJSONDecoder(file, int(stat.Size())).Decode(into)
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
