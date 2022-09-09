package vm

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	apiTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
	vaultmocks "github.com/hashicorp/consul-api-gateway/internal/vault/mocks"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/go-hclog"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

func testConsul(t *testing.T) *api.Client {
	t.Helper()

	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.Peering = nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = consulSrv.Stop()
	})
	consulSrv.WaitForLeader(t)

	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	consul, err := api.NewClient(cfg)
	require.NoError(t, err)
	return consul
}

func registerProxyService(t *testing.T, name string, port int, consul *api.Client) {
	t.Helper()

	require.NoError(t, consul.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Kind: api.ServiceKindConnectProxy,
		ID:   name,
		Name: name,
		Port: port,
		Proxy: &api.AgentServiceConnectProxyConfig{
			DestinationServiceName: name,
			LocalServiceAddress:    "127.0.0.1",
			LocalServicePort:       port,
		},
		Address: "127.0.0.1",
	}))
}

func registerBasicService(t *testing.T, name string, port int, consul *api.Client) {
	t.Helper()

	require.NoError(t, consul.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Kind:    api.ServiceKindTypical,
		ID:      name,
		Name:    name,
		Port:    port,
		Address: "127.0.0.1",
	}))
}

func TestValidateTCPRoute(t *testing.T) {
	consul := testConsul(t)
	registerProxyService(t, "service-1", 1234, consul)
	registerProxyService(t, "service-2", 1235, consul)
	registerBasicService(t, "service-3", 1236, consul)

	validator := NewValidator(hclog.NewNullLogger(), nil, consul)

	tests := []struct {
		name  string
		route *v1.TCPRoute
		want  []string
	}{{
		name: "pass",
		route: &v1.TCPRoute{
			Services: []v1.TCPService{{
				Name: "service-1",
			}, {
				Name: "service-2",
			}},
		},
	}, {
		name: "non-connect",
		route: &v1.TCPRoute{
			Services: []v1.TCPService{{
				Name: "service-1",
			}, {
				Name: "service-2",
			}, {
				Name: "service-3",
			}},
		},
		want: []string{
			`service "service-3" is not connect enabled`,
		},
	}, {
		name: "not-found",
		route: &v1.TCPRoute{
			Services: []v1.TCPService{{
				Name: "service-1",
			}, {
				Name: "service-2",
			}, {
				Name: "service-4",
			}},
		},
		want: []string{
			`no service "service-4" found`,
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTCPRoute(context.Background(), tt.route)
			if tt.want == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				for _, w := range tt.want {
					require.Contains(t, err.Error(), w)
				}
			}
		})
	}
}

func TestValidateHTTPRoute(t *testing.T) {
	consul := testConsul(t)
	registerProxyService(t, "service-1", 1234, consul)
	registerProxyService(t, "service-2", 1235, consul)
	registerBasicService(t, "service-3", 1235, consul)

	validator := NewValidator(hclog.NewNullLogger(), nil, consul)

	tests := []struct {
		name  string
		route *v1.HTTPRoute
		want  []string
	}{{
		name: "pass",
		route: &v1.HTTPRoute{
			Rules: []v1.HTTPRouteRule{{
				Services: []v1.HTTPService{{
					Name: "service-1",
				}, {
					Name: "service-2",
				}},
			}},
		},
	}, {
		name: "pass-two",
		route: &v1.HTTPRoute{
			Rules: []v1.HTTPRouteRule{{
				Services: []v1.HTTPService{{
					Name: "service-1",
				}},
			}, {
				Services: []v1.HTTPService{{
					Name: "service-2",
				}},
			}},
		},
	}, {
		name: "non-connect",
		route: &v1.HTTPRoute{
			Rules: []v1.HTTPRouteRule{{
				Services: []v1.HTTPService{{
					Name: "service-1",
				}, {
					Name: "service-2",
				}, {
					Name: "service-3",
				}},
			}},
		},
		want: []string{
			`service "service-3" is not connect enabled`,
		},
	}, {
		name: "not-found",
		route: &v1.HTTPRoute{
			Rules: []v1.HTTPRouteRule{{
				Services: []v1.HTTPService{{
					Name: "service-1",
				}, {
					Name: "service-2",
				}, {
					Name: "service-4",
				}},
			}},
		},
		want: []string{
			`no service "service-4" found`,
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateHTTPRoute(context.Background(), tt.route)
			if tt.want == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				for _, w := range tt.want {
					require.Contains(t, err.Error(), w)
				}
			}
		})
	}
}

func TestValidateGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	vault := vaultmocks.NewMockKVClient(ctrl)

	vault.EXPECT().Get(context.Background(), "certificate").Return(&vaultapi.KVSecret{
		Data: map[string]interface{}{
			"server.key":  "",
			"server.cert": "",
		},
	}, nil)
	vault.EXPECT().Get(context.Background(), "certificate-no-key").Return(&vaultapi.KVSecret{
		Data: map[string]interface{}{
			"server.cert": "",
		},
	}, nil)
	vault.EXPECT().Get(context.Background(), "certificate-no-chain").Return(&vaultapi.KVSecret{
		Data: map[string]interface{}{
			"server.key": "",
		},
	}, nil)
	vault.EXPECT().Get(context.Background(), "certificate-error").Return(nil, errors.New("expected"))

	validator := NewValidator(hclog.NewNullLogger(), vault, nil)

	tests := []struct {
		name    string
		gateway *v1.Gateway
		want    []string
	}{
		{
			name: "tls",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port:     1,
					Protocol: v1.ListenerProtocolHttp,
					Tls: &v1.TLSConfiguration{
						MinVersion:   apiTesting.StringPtr("TLSv1_3"),
						MaxVersion:   apiTesting.tringPtr("TLS_FOO"),
						CipherSuites: []string{"BAR"},
					},
				}},
			},
			want: []string{
				"configuring TLS cipher suites is only supported for TLS 1.2 and earlier",
				`invalid TLS version "TLS_FOO"`,
				`unsupported TLS cipher suite "BAR"`,
				"certificates must be specified if TLS is enabled",
			},
		}, {
			name: "listener-conflicts",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port: 1,
				}, {
					Port: 1,
				}},
			},
			want: []string{
				`name "" conflicts`,
				`port 1 conflicts`,
			},
		}, {
			name: "pass",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port: 1,
				}},
			},
		}, {
			name: "no-key-vault",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port: 1,
					Tls: &v1.TLSConfiguration{
						Certificates: []v1.Certificate{{
							Vault: &v1.VaultCertificate{
								Path:            "certificate-no-key",
								PrivateKeyField: "server.key",
								ChainField:      "server.cert",
							},
						}},
					},
				}},
			},
			want: []string{
				`invalid Vault field "server.key" for certificate private key`,
			},
		}, {
			name: "no-chain-vault",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port: 1,
					Tls: &v1.TLSConfiguration{
						Certificates: []v1.Certificate{{
							Vault: &v1.VaultCertificate{
								Path:            "certificate-no-chain",
								PrivateKeyField: "server.key",
								ChainField:      "server.cert",
							},
						}},
					},
				}},
			},
			want: []string{
				`invalid Vault field "server.cert" for certificate chain`,
			},
		}, {
			name: "error-vault",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port: 1,
					Tls: &v1.TLSConfiguration{
						Certificates: []v1.Certificate{{
							Vault: &v1.VaultCertificate{
								Path:            "certificate-error",
								PrivateKeyField: "server.key",
								ChainField:      "server.cert",
							},
						}},
					},
				}},
			},
			want: []string{
				`unable to retrieve Vault certificate`,
			},
		}, {
			name: "pass-vault",
			gateway: &v1.Gateway{
				Listeners: []v1.Listener{{
					Port: 1,
					Tls: &v1.TLSConfiguration{
						Certificates: []v1.Certificate{{
							Vault: &v1.VaultCertificate{
								Path:            "certificate",
								PrivateKeyField: "server.key",
								ChainField:      "server.cert",
							},
						}},
					},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateGateway(context.Background(), tt.gateway)
			if tt.want == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				for _, w := range tt.want {
					require.Contains(t, err.Error(), w)
				}
			}
		})
	}
}
