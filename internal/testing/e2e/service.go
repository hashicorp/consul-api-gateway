package e2e

import (
	"bytes"
	"context"
	"errors"
	"html/template"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul-api-gateway/internal/common"
	serviceResolver "github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul/api"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	envoyImage            = "envoyproxy/envoy:v1.19-latest"
	bootstrapJSONTemplate = `{
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
			"listeners": [{
				"name": "static",
				"address": {
					"socket_address": {
						"address": "127.0.0.1",
						"port_value": 19001
					}
				},
				"filter_chains": [{
					"filters": [{
						"name": "envoy.filters.network.http_connection_manager",
						"typed_config": {
							"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
							"stat_prefix": "edge",
							"http_filters": [{
								"name": "envoy.filters.http.router"
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
					"type": "{{ .AddressType }}",
					"transport_socket": {
						"name": "tls",
						"typed_config": {
							"@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
							"common_tls_context": {
								"validation_context": {
									"trusted_ca": {
										"filename": "/ca/tls.crt"
									}
								}
							}
						}
					},
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

var (
	bootstrapTemplate *template.Template
)

func init() {
	bootstrapTemplate = template.Must(template.New("bootstrap").Parse(bootstrapJSONTemplate))
}

type bootstrapArgs struct {
	ID            string
	AddressType   string
	ConsulAddress string
	Token         string
	ConsulXDSPort int
}

// DeployMeshService deploys an envoy proxy with roughly the same logic that consul-k8s uses
// in its connect-inject registration
func DeployMeshService(ctx context.Context, cfg *envconf.Config) (*core.Service, error) {
	servicePort := 8080
	namespace := Namespace(ctx)
	name := envconf.RandomName("mesh", 16)
	client := ConsulClient(ctx)
	consulPort := ConsulGRPCPort(ctx)
	token := ConsulMasterToken(ctx)
	consulAddress := HostRoute(ctx)
	proxyServiceName := name + "-proxy"

	resourcesClient := cfg.Client().Resources(namespace)

	configMap, err := meshServiceConfigMap(name, namespace, proxyServiceName, token, consulAddress, consulPort)
	if err != nil {
		return nil, err
	}

	if err := resourcesClient.Create(ctx, configMap); err != nil {
		return nil, err
	}

	deployment := meshDeployment(name, namespace, servicePort)
	if err := resourcesClient.Create(ctx, deployment); err != nil {
		return nil, err
	}

	service := meshService(deployment, servicePort)
	if err := resourcesClient.Create(ctx, service); err != nil {
		return nil, err
	}

	pod := &core.Pod{}
	err = backoff.Retry(func() error {
		list := &core.PodList{}
		if err := resourcesClient.List(ctx, list, resources.WithLabelSelector(meta.FormatLabelSelector(&meta.LabelSelector{
			MatchLabels: deployment.Labels,
		}))); err != nil {
			return err
		}

		if len(list.Items) == 0 {
			return errors.New("no pod created yet")
		}
		pod = &list.Items[0]

		if pod.Status.PodIP == "" {
			return errors.New("no assigned ip yet")
		}
		return nil
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 20), ctx))
	if err != nil {
		return nil, err
	}

	registration := &api.AgentServiceRegistration{
		ID:   name,
		Name: name,
		Port: 19001,
		Meta: map[string]string{
			serviceResolver.MetaKeyKubeServiceName: name,
			serviceResolver.MetaKeyKubeNS:          namespace,
		},
		Address: pod.Status.PodIP,
	}

	if err := client.Agent().ServiceRegisterOpts(registration, (&api.ServiceRegisterOpts{}).WithContext(ctx)); err != nil {
		return nil, err
	}

	_, _, err = client.ConfigEntries().Set(&api.ServiceConfigEntry{
		Kind:     api.ServiceDefaults,
		Name:     name,
		Protocol: "http",
	}, nil)
	if err != nil {
		return nil, err
	}

	proxyRegistration := &api.AgentServiceRegistration{
		Kind: api.ServiceKind(api.ServiceKindConnectProxy),
		ID:   proxyServiceName,
		Name: proxyServiceName,
		Port: servicePort,
		Meta: map[string]string{
			serviceResolver.MetaKeyKubeServiceName: name,
			serviceResolver.MetaKeyKubeNS:          namespace,
		},
		Proxy: &api.AgentServiceConnectProxyConfig{
			DestinationServiceName: name,
			LocalServiceAddress:    "127.0.0.1",
			LocalServicePort:       19001,
		},
		Address: pod.Status.PodIP,
	}

	if err := client.Agent().ServiceRegisterOpts(proxyRegistration, (&api.ServiceRegisterOpts{}).WithContext(ctx)); err != nil {
		return nil, err
	}
	return service, nil
}

func meshServiceConfigMap(name, namespace, proxyServiceName, token, consulAddress string, consulPort int) (*core.ConfigMap, error) {
	config, err := meshServiceConfig(proxyServiceName, token, consulAddress, consulPort)
	if err != nil {
		return nil, err
	}

	return &core.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"bootstrap.json": config,
		},
	}, nil
}

func meshServiceConfig(name, token, consulAddress string, consulPort int) (string, error) {
	var template bytes.Buffer
	if err := bootstrapTemplate.Execute(&template, &bootstrapArgs{
		ID:            name,
		AddressType:   common.AddressTypeForAddress(consulAddress),
		Token:         token,
		ConsulAddress: consulAddress,
		ConsulXDSPort: consulPort,
	}); err != nil {
		return "", err
	}
	return template.String(), nil
}

func meshService(deployment *apps.Deployment, port int) *core.Service {
	return &core.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
			Labels:    deployment.Labels,
		},
		Spec: core.ServiceSpec{
			Selector: deployment.Labels,
			Ports: []core.ServicePort{{
				Name:     "port",
				Protocol: core.ProtocolTCP,
				Port:     int32(port),
			}},
		},
	}
}

func meshDeployment(name, namespace string, port int) *apps.Deployment {
	labels := map[string]string{
		"deployment": name,
		"type":       "mesh-service",
	}
	return &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Selector: &meta.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels:    labels,
				},
				Spec: core.PodSpec{
					Volumes: []core.Volume{{
						Name: "ca",
						VolumeSource: core.VolumeSource{
							Secret: &core.SecretVolumeSource{
								SecretName: "consul-ca-cert",
							},
						},
					}, {
						Name: "config",
						VolumeSource: core.VolumeSource{
							ConfigMap: &core.ConfigMapVolumeSource{
								LocalObjectReference: core.LocalObjectReference{
									Name: name,
								},
							},
						},
					}},
					Containers: []core.Container{
						{
							Name:  "envoy",
							Image: envoyImage,
							Ports: []core.ContainerPort{{
								Name:          "port",
								Protocol:      "TCP",
								ContainerPort: int32(port),
							}},
							VolumeMounts: []core.VolumeMount{{
								Name:      "config",
								MountPath: "/config",
							}, {
								Name:      "ca",
								MountPath: "/ca",
								ReadOnly:  true,
							}},
							Command: []string{
								"envoy",
								"-c",
								"/config/bootstrap.json",
								"-l",
								"trace",
							},
						},
					},
				},
			},
		},
	}
}
