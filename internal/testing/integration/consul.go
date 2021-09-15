package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/polar/internal/testing"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	consulImage          = "hashicorp/consul:1.10.2"
	configTemplateString = `
{
	"acl": {
		"enabled": true,
		"default_policy": "deny"
	},
	"server": true,
	"bootstrap": true,
	"bootstrap_expect": 1,
	"disable_update_check": true,
	"skip_leave_on_interrupt": true,
	"addresses": {
    "https": "0.0.0.0",
    "grpc": "0.0.0.0"
  },
	"ports": {
		"https": {{ .HTTPSPort }},
		"grpc": {{ .GRPCPort }}
	},
	"data_dir": "/data",
	"ca_file": "/ca/tls.crt",
	"cert_file": "/cert/tls.crt",
	"key_file": "/cert/tls.key",
	"connect": {
		"enabled": true
	},
	"ui": true
}
`
)

type consulTestContext struct{}

var (
	consulTestContextKey = consulTestContext{}

	configTemplate *template.Template
)

func init() {
	configTemplate = template.Must(template.New("config").Parse(configTemplateString))
}

type consulTestEnvironment struct {
	ca           []byte
	consulClient *api.Client
	token        string
	policy       *api.ACLPolicy
	httpPort     int
	grpcPort     int
}

func CreateTestConsulContainer(name, namespace string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Print("Creating Consul container")

		clusterVal := ctx.Value(kindContextKey(name))
		if clusterVal == nil {
			return ctx, fmt.Errorf("context cluster is nil")
		}
		cluster := clusterVal.(*kindCluster)
		httpsPort := cluster.httpsPort
		grpcPort := cluster.grpcPort

		rootCA, err := testing.GenerateSignedCertificate(testing.GenerateCertificateOptions{
			IsCA: true,
		})
		if err != nil {
			return nil, err
		}
		serverCert, err := testing.GenerateSignedCertificate(testing.GenerateCertificateOptions{
			CA:        rootCA,
			ExtraSANs: []string{"localhost", "host.docker.internal"},
		})
		if err != nil {
			return nil, err
		}
		clientCert, err := testing.GenerateSignedCertificate(testing.GenerateCertificateOptions{
			CA: rootCA,
		})
		if err != nil {
			return nil, err
		}

		serverCertSecret := consulServerCertSecret(namespace, serverCert)
		if err := cfg.Client().Resources().Create(ctx, serverCertSecret); err != nil {
			return nil, err
		}

		caCertSecret := consulCASecret(namespace, rootCA)
		if err := cfg.Client().Resources().Create(ctx, caCertSecret); err != nil {
			return nil, err
		}

		consulConfig, err := consulConfigMap(namespace, httpsPort, grpcPort)
		if err != nil {
			return nil, err
		}
		if err := cfg.Client().Resources().Create(ctx, consulConfig); err != nil {
			return nil, err
		}

		deployment := consulDeployment(namespace, httpsPort, grpcPort)
		if err := cfg.Client().Resources().Create(ctx, deployment); err != nil {
			return nil, err
		}

		consulClient, err := api.NewClient(&api.Config{
			Address: fmt.Sprintf("localhost:%d", httpsPort),
			Scheme:  "https",
			TLSConfig: api.TLSConfig{
				CAPem:   rootCA.CertBytes,
				CertPEM: clientCert.CertBytes,
				KeyPEM:  clientCert.PrivateKeyBytes,
			},
		})
		if err != nil {
			return nil, err
		}

		// wait for the consul instance to start up
		log.Print("Waiting for consul to be ready")
		err = backoff.Retry(func() error {
			_, meta, err := consulClient.Catalog().Nodes(nil)
			if err != nil {
				return err
			}
			if !meta.KnownLeader {
				return errors.New("no known consul leader")
			}
			return nil
		}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 20), ctx))
		if err != nil {
			return nil, err
		}

		env := &consulTestEnvironment{
			ca:           rootCA.CertBytes,
			consulClient: consulClient,
			httpPort:     httpsPort,
			grpcPort:     grpcPort,
		}
		return context.WithValue(ctx, consulTestContextKey, env), nil
	}
}

func consulServerCertSecret(namespace string, serverCert *testing.CertificateInfo) client.Object {
	return &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-server-cert",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			core.TLSCertKey:       serverCert.CertBytes,
			core.TLSPrivateKeyKey: serverCert.PrivateKeyBytes,
		},
		Type: core.SecretTypeTLS,
	}
}

func consulCASecret(namespace string, caCert *testing.CertificateInfo) client.Object {
	return &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-ca-cert",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			core.TLSCertKey: caCert.CertBytes,
		},
		Type: core.SecretTypeOpaque,
	}
}

func consulConfig(httpsPort, grpcPort int) (string, error) {
	var template bytes.Buffer
	if err := configTemplate.Execute(&template, &struct {
		HTTPSPort int
		GRPCPort  int
	}{
		HTTPSPort: httpsPort,
		GRPCPort:  grpcPort,
	}); err != nil {
		return "", err
	}
	return template.String(), nil
}

func consulConfigMap(namespace string, httpsPort, grpcPort int) (client.Object, error) {
	config, err := consulConfig(httpsPort, grpcPort)
	if err != nil {
		return nil, err
	}

	return &core.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"consul.json": config,
		},
	}, nil
}

func consulDeployment(namespace string, httpsPort, grpcPort int) client.Object {
	labels := map[string]string{
		"deployment": "consul-test-server",
	}
	return &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Selector: &meta.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Name:      "consul",
					Namespace: namespace,
					Labels:    labels,
				},
				Spec: core.PodSpec{
					ServiceAccountName: "polar",
					Volumes: []core.Volume{{
						Name: "data",
						VolumeSource: core.VolumeSource{
							EmptyDir: &core.EmptyDirVolumeSource{},
						},
					}, {
						Name: "ca",
						VolumeSource: core.VolumeSource{
							Secret: &core.SecretVolumeSource{
								SecretName: "consul-ca-cert",
							},
						},
					}, {
						Name: "cert",
						VolumeSource: core.VolumeSource{
							Secret: &core.SecretVolumeSource{
								SecretName: "consul-server-cert",
							},
						},
					}, {
						Name: "config",
						VolumeSource: core.VolumeSource{
							ConfigMap: &core.ConfigMapVolumeSource{
								LocalObjectReference: core.LocalObjectReference{
									Name: "consul-config",
								},
							},
						},
					}},
					Containers: []core.Container{
						{
							Name:  "consul",
							Image: consulImage,
							Ports: []core.ContainerPort{{
								Name:          "https",
								Protocol:      "TCP",
								ContainerPort: int32(httpsPort),
								HostPort:      int32(httpsPort),
							}, {
								Name:          "grpc",
								Protocol:      "TCP",
								ContainerPort: int32(grpcPort),
								HostPort:      int32(grpcPort),
							}},
							VolumeMounts: []core.VolumeMount{{
								Name:      "data",
								MountPath: "/data",
							}, {
								Name:      "cert",
								MountPath: "/cert",
								ReadOnly:  true,
							}, {
								Name:      "ca",
								MountPath: "/ca",
								ReadOnly:  true,
							}, {
								Name:      "config",
								MountPath: "/config",
								ReadOnly:  true,
							}},
							Command: []string{
								"consul",
								"agent",
								"-config-file",
								"/config/consul.json",
								"-config-format",
								"json",
							},
						},
					},
				},
			},
		},
	}
}

func ConsulClient(ctx context.Context) *api.Client {
	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		panic("must run this with an integration test that has called CreateTestConsul")
	}
	return consulEnvironment.(*consulTestEnvironment).consulClient
}

func ConsulMasterToken(ctx context.Context) string {
	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		panic("must run this with an integration test that has called CreateTestConsul")
	}
	return consulEnvironment.(*consulTestEnvironment).token
}

func ConsulGRPCPort(ctx context.Context) int {
	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		panic("must run this with an integration test that has called CreateTestConsul")
	}
	return consulEnvironment.(*consulTestEnvironment).grpcPort
}

func ConsulHTTPPort(ctx context.Context) int {
	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		panic("must run this with an integration test that has called CreateTestConsul")
	}
	return consulEnvironment.(*consulTestEnvironment).httpPort
}

func CreateConsulACLPolicy(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Creating Consul ACL Policy")

	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		return ctx, nil
	}
	env := consulEnvironment.(*consulTestEnvironment)
	token, _, err := env.consulClient.ACL().Bootstrap()
	if err != nil {
		return nil, err
	}
	log.Printf("Consul master token: %s", token.SecretID)
	policy, _, err := env.consulClient.ACL().PolicyCreate(adminPolicy(), &api.WriteOptions{
		Token: token.SecretID,
	})
	if err != nil {
		return nil, err
	}
	env.token = token.SecretID
	env.policy = policy
	return ctx, nil
}

func CreateConsulAuthMethod(namespace string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Print("Creating Consul ACL Auth Method")

		consulEnvironment := ctx.Value(consulTestContextKey)
		if consulEnvironment == nil {
			return ctx, nil
		}
		env := consulEnvironment.(*consulTestEnvironment)
		_, _, err := env.consulClient.ACL().RoleCreate(polarConsulRole(namespace, env.policy.ID), &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		_, _, err = env.consulClient.ACL().AuthMethodCreate(polarConsulAuthMethod(ClusterName(ctx), K8sServiceToken(ctx), cfg.Client().RESTConfig()), &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		_, _, err = env.consulClient.ACL().BindingRuleCreate(polarConsulBindingRule(), &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

func polarConsulAuthMethod(name, token string, k8sConfig *rest.Config) *api.ACLAuthMethod {
	return &api.ACLAuthMethod{
		Type: "kubernetes",
		Name: "polar",
		Config: map[string]interface{}{
			"Host":              fmt.Sprintf("https://%s-control-plane:6443", name),
			"CACert":            string(k8sConfig.CAData),
			"ServiceAccountJWT": token,
		},
	}
}

func polarConsulBindingRule() *api.ACLBindingRule {
	return &api.ACLBindingRule{
		AuthMethod: "polar",
		BindType:   api.BindingRuleBindTypeRole,
		BindName:   "polar",
		Selector:   `serviceaccount.name=="polar"`,
	}
}

func polarConsulRole(namespace, policyID string) *api.ACLRole {
	return &api.ACLRole{
		Name: "polar",
		Policies: []*api.ACLLink{
			{
				ID:   policyID,
				Name: "polar",
			},
		},
	}
}

func adminPolicy() *api.ACLPolicy {
	return &api.ACLPolicy{
		Name: "polar",
		Rules: `
node_prefix "" { policy = "write" }
service_prefix "" { policy = "write" }
agent_prefix "" { policy = "write" }
event_prefix "" { policy = "write" }
query_prefix "" { policy = "write" }
session_prefix "" { policy = "write" }
operator = "write"
acl = "write"
keyring = "write"
`,
	}
}
