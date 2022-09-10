package vm

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"text/template"

	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/stretchr/testify/require"
)

var (
	tcpBootstrapTemplate  *template.Template
	httpBootstrapTemplate *template.Template
)

func init() {
	tcpBootstrapTemplate = template.Must(template.New("tcp-bootstrap").Parse(tcpBootstrapJSONTemplate))
	httpBootstrapTemplate = template.Must(template.New("http-bootstrap").Parse(httpBootstrapJSONTemplate))
}

type bootstrapArgs struct {
	ID            string
	Token         string
	ConsulXDSPort int
	Port          int
	CertFile      string
}

type ProxyTarget struct {
	Name      string
	Port      int
	ProxyPort int
}

func (c *Controller) runProxyTarget(t *testing.T, name string, template *template.Template) *ProxyTarget {
	t.Helper()

	envoyBinary, err := exec.LookPath("envoy")
	require.NoError(t, err)

	name, path, port, proxyPort := c.renderBootstrap(t, name, template)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, envoyBinary, "-c", path, "--log-level", "error")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		cmd.Process.Kill()
		cancel()
	})

	return &ProxyTarget{
		Name:      name,
		Port:      port,
		ProxyPort: proxyPort,
	}
}

func (c *Controller) renderBootstrap(t *testing.T, name string, template *template.Template) (string, string, int, int) {
	t.Helper()

	ports := freeport.MustTake(2)
	port := ports[0]
	proxyPort := ports[1]

	file, err := os.CreateTemp("", "bootstrap")
	require.NoError(t, err)

	path := file.Name()
	require.NoError(t, file.Close())

	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	var data bytes.Buffer
	require.NoError(t, template.Execute(&data, &bootstrapArgs{
		ID:            name,
		Token:         c.Consul.Token,
		ConsulXDSPort: c.Consul.XDSPort,
		Port:          port,
		// interestingly the gRPC port using testutil is not TLS
		CertFile: c.Consul.ConnectCert,
	}))
	require.NoError(t, os.WriteFile(path, data.Bytes(), 0644))

	return name, path, port, proxyPort
}

const (
	httpBootstrapJSONTemplate = `{
	"admin": {},
	"node": {
		"cluster": "{{ .ID }}",
		"id": "{{ .ID }}",
		"metadata": {
			"namespace": "default"
		}
	},
	"static_resources": {
		"listeners": [{
			"name": "static",
			"address": {
				"socket_address": {
					"address": "127.0.0.1",
					"port_value": {{ .Port }}
				}
			},
			"filter_chains": [{
				"filters": [{
					"name": "envoy.filters.network.http_connection_manager",
					"typed_config": {
						"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
						"stat_prefix": "edge",
						"http_filters": [{
							"name": "envoy.filters.http.router",
							"typed_config": {
								"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
							}
						}],
						"route_config": {
							"virtual_hosts": [{
								"name": "direct_response",
								"domains": ["*"],
								"routes": [{
									"match": {
										"prefix": "/"
									},
									"direct_response": {
										"status": 200,
										"body": {
											"inline_string": "{{ .ID }}"
										}
									}
								}]
							}]	
						}
					}
				}]
			}]
		}],
		"clusters": [
			{
				"name": "consul-server",
				"ignore_health_on_host_removal": false,
				"connect_timeout": "1s",
				"type": "STATIC",
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
												"address": "127.0.0.1",
												"port_value": {{ .ConsulXDSPort }}
											}
										}
									}
								}
							]
						}
					]
				}
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
	tcpBootstrapJSONTemplate = `{
	"admin": {},
	"node": {
		"cluster": "{{ .ID }}",
		"id": "{{ .ID }}",
		"metadata": {
			"namespace": "default"
		}
	},
	"static_resources": {
		"listeners": [{
			"name": "static",
			"address": {
				"socket_address": {
					"address": "127.0.0.1",
					"port_value": {{ .Port }}
				}
			},
			"filter_chains": [{
				"filters": [{
					"name": "envoy.filters.network.direct_response",
					"typed_config": {
						"@type": "type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config",
						"response": {
							"inline_string": "{{ .ID }}"
						}
					}
				}]
			}]
		}],
		"clusters": [
			{
				"name": "consul-server",
				"ignore_health_on_host_removal": false,
				"connect_timeout": "1s",
				"type": "STATIC",
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
												"address": "127.0.0.1",
												"port_value": {{ .ConsulXDSPort }}
											}
										}
									}
								}
							]
						}
					]
				}
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
)
