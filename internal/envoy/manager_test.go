package envoy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

var (
	generate bool
)

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

func TestManagerRenderBootstrap(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		config    ManagerConfig
		sdsConfig string
	}{{
		name: "empty",
	}, {
		name: "basic",
		config: ManagerConfig{
			ID:            "6f164d52-7846-45d1-97b1-fc984795572b",
			ConsulAddress: "yahoo.com",
			ConsulXDSPort: 4544,
			Token:         "ac80a2e4-3e37-464f-bb2d-2d2be43be022",
			LogLevel:      "info",
		},
	}, {
		name: "tls",
		config: ManagerConfig{
			ID:            "feaf6c11-f46f-4869-ba53-b0cc07f09659",
			ConsulCA:      "/file/path/to/ca",
			ConsulAddress: "google.com",
			ConsulXDSPort: 4545,
			Token:         "b19562d6-c563-4b7e-a2d6-32c44b48b7b1",
			LogLevel:      "debug",
		},
	}, {
		name:      "sds",
		sdsConfig: `{"kv":"here"}`,
	}} {
		t.Run(test.name, func(t *testing.T) {
			directory, err := os.MkdirTemp("", "polar-test")
			require.NoError(t, err)
			defer os.RemoveAll(directory)

			filePath := path.Join(directory, "bootstrap.json")
			test.config.BootstrapFilePath = filePath

			manager := NewManager(hclog.NewNullLogger(), test.config)
			err = manager.RenderBootstrap(test.sdsConfig)
			require.NoError(t, err)

			data, err := os.ReadFile(filePath)
			require.NoError(t, err)

			var expected string
			expectedFileName := fmt.Sprintf("%s.golden.json", test.name)
			if generate {
				var buffer bytes.Buffer
				err := json.Indent(&buffer, data, "", "  ")
				require.NoError(t, err)
				err = os.WriteFile(path.Join("testdata", expectedFileName), buffer.Bytes(), 0644)
				require.NoError(t, err)
				expected = buffer.String()
			} else {
				data, err := os.ReadFile(path.Join("testdata", expectedFileName))
				require.NoError(t, err)
				expected = string(data)
			}

			require.JSONEq(t, expected, string(data))
		})
	}
}
