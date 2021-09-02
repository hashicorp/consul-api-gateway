package controllers

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var generate bool

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

func TestDeploymentFor(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
	}{{
		name: "basic",
	}, {
		name: "annotations",
	}, {
		name: "tls-cert",
	}} {
		t.Run(test.name, func(t *testing.T) {
			file, err := os.OpenFile(path.Join("testdata", fmt.Sprintf("%s.yaml", test.name)), os.O_RDONLY, 0644)
			require.NoError(t, err)
			defer file.Close()

			stat, err := file.Stat()
			require.NoError(t, err)

			gw := &gateway.Gateway{}
			err = yaml.NewYAMLOrJSONDecoder(file, int(stat.Size())).Decode(gw)
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
			err = serializer.Encode(DeploymentFor(gw), &buffer)
			// data, err := json.MarshalIndent(DeploymentFor(gw), "", "  ")
			require.NoError(t, err)

			var expected string
			if generate {
				expected = buffer.String()
				err := os.WriteFile(path.Join("testdata", fmt.Sprintf("%s.golden.yaml", test.name)), buffer.Bytes(), 0644)
				require.NoError(t, err)
			} else {
				data, err := os.ReadFile(path.Join("testdata", fmt.Sprintf("%s.golden.yaml", test.name)))
				require.NoError(t, err)
				expected = string(data)
			}

			require.Equal(t, expected, buffer.String())
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
