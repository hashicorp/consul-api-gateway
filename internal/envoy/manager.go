package envoy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"text/template"

	"github.com/hashicorp/polar/internal/common"

	"github.com/hashicorp/go-hclog"
)

const (
	logFormatString = `{"timestamp":"%Y-%m-%d %T.%e","thread":"%t","level":"%l","name":"%n","source":"%g:%#","message":"%v"}`
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
	AddressType   string
}

func init() {
	_, err := bootstrapTemplate.Parse(bootstrapJSONTemplate)
	if err != nil {
		panic(err)
	}
}

type ManagerConfig struct {
	ID                string
	ConsulCA          string
	ConsulAddress     string
	ConsulXDSPort     int
	Token             string
	BootstrapFilePath string
	LogLevel          string
}

type Manager struct {
	logger hclog.Logger

	ManagerConfig
}

func NewManager(logger hclog.Logger, config ManagerConfig) *Manager {
	return &Manager{
		logger:        logger,
		ManagerConfig: config,
	}
}

func (m *Manager) Run(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "envoy", "-l", m.LogLevel, "--log-format", logFormatString, "-c", m.BootstrapFilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if errors.Is(err, context.Canceled) {
		// we intentionally canceled the context, just return
		return nil
	}
	return err
}

func (m *Manager) RenderBootstrap(sdsConfig string) error {
	var bootstrapConfig bytes.Buffer
	if err := bootstrapTemplate.Execute(&bootstrapConfig, &bootstrapArgs{
		SDSCluster:    sdsConfig,
		ID:            m.ID,
		ConsulCA:      m.ConsulCA,
		ConsulAddress: m.ConsulAddress,
		ConsulXDSPort: m.ConsulXDSPort,
		AddressType:   common.AddressTypeForAddress(m.ConsulAddress),
		Token:         m.Token,
	}); err != nil {
		return err
	}

	return os.WriteFile(m.BootstrapFilePath, bootstrapConfig.Bytes(), 0600)
}

const bootstrapJSONTemplate = `{
	"admin": {
    "access_log_path": "/dev/null",
    "address": {
      "socket_address": {
        "address": "127.0.0.1",
        "port_value": 19000
      }
    }
  },
  "node": {
    "cluster": "{{ .ID }}",
    "id": "{{ .ID }}"
  },
  "static_resources": {
    "clusters": [
			{
        "name": "self_admin",
        "ignore_health_on_host_removal": false,
        "connect_timeout": "5s",
        "type": "STATIC",
        "http_protocol_options": {},
        "loadAssignment": {
          "clusterName": "self_admin",
          "endpoints": [
            {
              "lbEndpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 19000
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
			},
      {
        "name": "consul-server",
        "ignore_health_on_host_removal": false,
        "connect_timeout": "1s",
        "type": "{{ .AddressType }}",
        {{- if ne .ConsulCA "" -}}
        "transport_socket": {
          "name": "tls",
          "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
            "common_tls_context": {
              "validation_context": {
                "trusted_ca": {
                  "filename": "{{ .ConsulCA }}"
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
    ],
		"listeners": [
      {
        "name": "envoy_ready_listener",
        "address": {
          "socket_address": {
            "address": "0.0.0.0",
            "port_value": 20000
          }
        },
        "filter_chains": [
          {
            "filters": [
              {
                "name": "envoy.filters.network.http_connection_manager",
                "typedConfig": {
                  "@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
                  "stat_prefix": "envoy_ready",
                  "codec_type": "HTTP1",
                  "route_config": {
                    "name": "self_admin_route",
                    "virtual_hosts": [
                      {
                        "name": "self_admin",
                        "domains": [
                          "*"
                        ],
                        "routes": [
                          {
                            "match": {
                              "path": "/ready"
                            },
                            "route": {
                              "cluster": "self_admin",
                              "prefix_rewrite": "/ready"
                            }
                          },
                          {
                            "match": {
                              "prefix": "/"
                            },
                            "direct_response": {
                              "status": 404
                            }
                          }
                        ]
                      }
                    ]
                  },
                  "http_filters": [
                    {
                      "name": "envoy.filters.http.router"
                    }
                  ]
                }
              }
            ]
          }
        ]
      }
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
