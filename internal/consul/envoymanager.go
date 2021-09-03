package consul

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/hashicorp/go-hclog"
)

var (
	bootstrapTemplate = template.New("bootstrap")
)

type bootstrapArgs struct {
	ID            string
	ConsulCA      string
	ConsulAddress string
	ConsulXDSPort int
	SDSCluster    string
	Token         string
}

func init() {
	_, err := bootstrapTemplate.Parse(bootstrapJSONTemplate)
	if err != nil {
		panic(err)
	}
}

type EnvoyManagerConfig struct {
	ID            string
	ConsulCA      string
	ConsulAddress string
	ConsulXDSPort int
	Token         string

	BootstrapFilePath string
}

type EnvoyManager struct {
	logger hclog.Logger
	ports  []NamedPort

	config EnvoyManagerConfig
}

func NewEnvoyManager(logger hclog.Logger, ports []NamedPort, config EnvoyManagerConfig) *EnvoyManager {
	return &EnvoyManager{
		logger: logger,
		ports:  ports,
		config: config,
	}
}

func (m *EnvoyManager) Run(ctx context.Context) error {
	cmd := m.command()
	err := exec.CommandContext(ctx, "sh", "-c", cmd).Run()
	if errors.Is(err, context.Canceled) {
		// we intentionally canceled the context, just return
		return nil
	}
	return err
}

func (m *EnvoyManager) command() string {
	// replace all this junk with something that actually uses envoy
	template := "sh -c \"while true; do printf 'HTTP/1.1 200 OK\nConnection: close\nContent-Length: %d\n\n%s\n' | nc -l %d; done\" &"
	commands := []string{}
	for _, port := range m.ports {
		message := fmt.Sprintf("Protocol: %s, Name: %s, Port: %d", port.Protocol, port.Name, port.Port)
		commands = append(commands, fmt.Sprintf(template, len(message)+1, message, port.Port))
	}
	commands = append(commands, "wait $(jobs -p)")
	return strings.Join(commands, "\n")
}

func (m *EnvoyManager) RenderBootstrap(manager *CertManager) error {
	config, err := manager.SDSConfig()
	if err != nil {
		return err
	}

	var bootstrapConfig bytes.Buffer
	if err := bootstrapTemplate.Execute(&bootstrapConfig, &bootstrapArgs{
		SDSCluster:    config,
		ID:            m.config.ID,
		ConsulCA:      m.config.ConsulCA,
		ConsulAddress: m.config.ConsulAddress,
		ConsulXDSPort: m.config.ConsulXDSPort,
		Token:         m.config.Token,
	}); err != nil {
		return err
	}

	return os.WriteFile(m.config.BootstrapFilePath, bootstrapConfig.Bytes(), 0600)
}

const bootstrapJSONTemplate = `{
  "node": {
    "cluster": "{{ .ID }}",
    "id": "{{ .ID }}"
  },
  "static_resources": {
    "clusters": [
      {
        "name": "consul-server",
        "ignore_health_on_host_removal": false,
        "connect_timeout": "1s",
        "type": "STATIC",
        {{- if ne .ConsulCA "" -}}
        "transport_socket": {
          "name": "tls",
          "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
            "common_tls_context": {
              "validation_context": {
                "trusted_ca": {
                  "file": "{{ .ConsulCA }}"
                }
              }
            }
          }
        },
        {{- end }}
        "http2_protocol_options": {},
        "loadAssignment": {
          "clusterName": "consul-server",
          "endpoints": [
            {
              "lbEndpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "{{ .ConsulAddress }}",
                        "port_value": {{ .ConsulXDSPort }}
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      },
			{{ .SDSCluster }}
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "ads": {},
      "resource_api_version": "V3"
    },
    "cds_config": {
      "ads": {},
      "resource_api_version": "V3"
    },
    "ads_config": {
      "api_type": "DELTA_GRPC",
      "transport_api_version": "V3",
      "grpc_services": {
        "initial_metadata": [
          {
            "key": "x-consul-token",
            "value": "{{ .Token }}"
          }
        ],
        "envoy_grpc": {
          "cluster_name": "consul-server"
        }
      }
    }
  }
}
`
