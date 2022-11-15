package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"

	"github.com/hashicorp/consul-api-gateway/internal/consul"

	"github.com/Masterminds/semver"
	"github.com/cenkalti/backoff"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/hashicorp/consul/api"

	"github.com/hashicorp/consul-api-gateway/internal/testing"
)

const (
	grpcConsulIncompatibleNameVersion = "1.14"
	defaultConsulImage                = "hashicorppreview/consul:1.14-dev"
	envvarConsulImage                 = envvarPrefix + "CONSUL_IMAGE"
	envvarConsulEnterpriseLicense     = "CONSUL_LICENSE"
	envvarConsulEnterpriseLicensePath = "CONSUL_LICENSE_PATH"
	envvarGRPCName                    = "CONSUL_GRPC_VAR_NAME"
	configTemplateString              = `
{
	"log_level": "trace",
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
    "{{ .GRPCVarName }}": "0.0.0.0"
  },
  "ports": {
    "https": {{ .HTTPSPort }},
    "{{ .GRPCVarName }}": {{ .GRPCPort }}
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

	configTemplate   *template.Template
	consulImage      = getEnvDefault(envvarConsulImage, defaultConsulImage)
	consulEntLicense = ""
)

func init() {
	configTemplate = template.Must(template.New("config").Parse(configTemplateString))
	var err error
	consulEntLicense, err = loadLicense("")
	if err != nil {
		panic(err)
	}
}

type consulTestEnvironment struct {
	ca                               []byte
	consulClient                     consul.Client
	token                            string
	policy                           *api.ACLPolicy
	httpPort                         int
	httpFlattenedPort                int
	httpReferenceGrantPort           int
	tcpReferenceGrantPort            int
	parentRefChangeFirstGatewayPort  int
	parentRefChangeSecondGatewayPort int
	grpcPort                         int
	extraHTTPPort                    int
	extraTCPPort                     int
	extraTCPTLSPort                  int
	extraTCPTLSPortTwo               int
	namespace                        string
	ip                               string
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
		httpFlattenedPort := cluster.httpsFlattenedPort
		httpReferenceGrantPort := cluster.httpsReferenceGrantPort
		tcpReferenceGrantPort := cluster.tcpReferenceGrantPort
		parentRefChangeFirstGatewayPort := cluster.parentRefChangeFirstGatewayPort
		parentRefChangeSecondGatewayPort := cluster.parentRefChangeSecondGatewayPort
		grpcPort := cluster.grpcPort
		extraTCPPort := cluster.extraTCPPort
		extraTCPTLSPort := cluster.extraTCPTLSPort
		extraTCPTLSPortTwo := cluster.extraTCPTLSPortTwo
		extraHTTPPort := cluster.extraHTTPPort

		rootCA, err := testing.GenerateSignedCertificate(testing.GenerateCertificateOptions{
			IsCA: true,
			Bits: 2048,
		})
		if err != nil {
			return nil, err
		}
		serverCert, err := testing.GenerateSignedCertificate(testing.GenerateCertificateOptions{
			CA:        rootCA,
			Bits:      2048,
			ExtraSANs: []string{"localhost", "host.docker.internal"}, // host route for docker on mac/windows
			ExtraIPs:  []net.IP{net.IPv4(172, 17, 0, 1)},             // host ip for docker bridge
		})
		if err != nil {
			return nil, err
		}
		clientCert, err := testing.GenerateSignedCertificate(testing.GenerateCertificateOptions{
			CA:   rootCA,
			Bits: 2048,
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
		log.Print("Consul is ready")

		ip, err := consulPodIP(ctx, cfg, deployment)
		if err != nil {
			return nil, err
		}

		env := &consulTestEnvironment{
			ca:                               rootCA.CertBytes,
			consulClient:                     testing.NewTestClient(consulClient),
			httpPort:                         httpsPort,
			httpFlattenedPort:                httpFlattenedPort,
			httpReferenceGrantPort:           httpReferenceGrantPort,
			tcpReferenceGrantPort:            tcpReferenceGrantPort,
			parentRefChangeFirstGatewayPort:  parentRefChangeFirstGatewayPort,
			parentRefChangeSecondGatewayPort: parentRefChangeSecondGatewayPort,
			grpcPort:                         grpcPort,
			extraHTTPPort:                    extraHTTPPort,
			extraTCPPort:                     extraTCPPort,
			extraTCPTLSPort:                  extraTCPTLSPort,
			extraTCPTLSPortTwo:               extraTCPTLSPortTwo,
			ip:                               ip,
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

func consulGRPCVarName() string {
	tagTokens := strings.Split(consulImage, ":")
	tag := tagTokens[len(tagTokens)-1]
	imageVersion, err := semver.NewVersion(tag)
	if err != nil {
		return "grpc"
	}
	breakingVersion, err := semver.NewVersion(grpcConsulIncompatibleNameVersion)
	if err != nil {
		return "grpc"
	}
	// we check major/minor directly since the semver check fails with the trailing -dev tag
	if breakingVersion.Major() > imageVersion.Major() || (breakingVersion.Major() == imageVersion.Major() && breakingVersion.Minor() > imageVersion.Minor()) {
		return "grpc"
	}
	return "grpc_tls"
}

func consulPodIP(ctx context.Context, cfg *envconf.Config, deployment *apps.Deployment) (string, error) {
	namespace := Namespace(ctx)
	resourcesClient := cfg.Client().Resources(namespace)

	pod := &core.Pod{}
	err := backoff.Retry(func() error {
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
		return "", err
	}
	return pod.Status.PodIP, nil
}

func consulConfig(httpsPort, grpcPort int) (string, error) {
	var template bytes.Buffer

	if err := configTemplate.Execute(&template, &struct {
		HTTPSPort   int
		GRPCPort    int
		GRPCVarName string
	}{
		HTTPSPort:   httpsPort,
		GRPCPort:    grpcPort,
		GRPCVarName: getEnvDefault(envvarGRPCName, consulGRPCVarName()),
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

func consulDeployment(namespace string, httpsPort, grpcPort int) *apps.Deployment {
	labels := map[string]string{
		"deployment": "consul-test-server",
	}
	var env []core.EnvVar
	if consulEntLicense != "" {
		env = append(env, core.EnvVar{
			Name:  envvarConsulEnterpriseLicense,
			Value: consulEntLicense,
		})
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
					ServiceAccountName: "consul-api-gateway",
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
							Env:   env,
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

func ConsulClient(ctx context.Context) consul.Client {
	return mustGetTestEnvironment(ctx).consulClient
}

func ConsulCA(ctx context.Context) string {
	return string(mustGetTestEnvironment(ctx).ca)

}

func ConsulInitialManagementToken(ctx context.Context) string {
	return mustGetTestEnvironment(ctx).token
}

func ConsulIP(ctx context.Context) string {
	return mustGetTestEnvironment(ctx).ip
}

func ConsulGRPCPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).grpcPort
}

func TCPPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).extraTCPPort
}

func TCPTLSPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).extraTCPTLSPort
}

func ExtraTCPTLSPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).extraTCPTLSPortTwo
}

func HTTPPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).extraHTTPPort
}

func HTTPFlattenedPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).httpFlattenedPort
}

func HTTPReferenceGrantPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).httpReferenceGrantPort
}

func TCPReferenceGrantPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).tcpReferenceGrantPort
}

func ParentRefChangeFirstGatewayPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).parentRefChangeFirstGatewayPort
}

func ParentRefChangeSecondGatewayPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).parentRefChangeSecondGatewayPort
}

func ConsulHTTPPort(ctx context.Context) int {
	return mustGetTestEnvironment(ctx).httpPort
}

func isConsulNamespaceMirroringOn() bool {
	return IsEnterprise()
}
func ConsulNamespace(ctx context.Context) string {
	if isConsulNamespaceMirroringOn() {
		//assume mirroring is on
		return Namespace(ctx)
	}
	return mustGetTestEnvironment(ctx).namespace
}

func mustGetTestEnvironment(ctx context.Context) *consulTestEnvironment {
	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		panic("must run this with an integration test that has called CreateTestConsul")
	}
	return consulEnvironment.(*consulTestEnvironment)
}

func CreateConsulACLPolicy(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Creating Consul ACL Policy")

	consulEnvironment := ctx.Value(consulTestContextKey)
	if consulEnvironment == nil {
		return ctx, nil
	}
	env := consulEnvironment.(*consulTestEnvironment)
	token, _, err := env.consulClient.Internal().ACL().Bootstrap()
	if err != nil {
		return nil, err
	}
	log.Printf("Consul initial management token: %s", token.SecretID)
	policy, _, err := env.consulClient.Internal().ACL().PolicyCreate(adminPolicy(), &api.WriteOptions{
		Token: token.SecretID,
	})
	if err != nil {
		return nil, err
	}
	env.token = token.SecretID
	env.policy = policy
	return ctx, nil
}

func CreateConsulAuthMethod() env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Print("Creating Consul ACL Auth Method")

		consulEnvironment := ctx.Value(consulTestContextKey)
		if consulEnvironment == nil {
			return ctx, nil
		}
		env := consulEnvironment.(*consulTestEnvironment)
		_, _, err := env.consulClient.Internal().ACL().RoleCreate(gatewayConsulRole(env.policy.ID), &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		_, _, err = env.consulClient.Internal().ACL().AuthMethodCreate(gatewayConsulAuthMethod(ClusterName(ctx), K8sServiceToken(ctx), cfg.Client().RESTConfig()), &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		_, _, err = env.consulClient.Internal().ACL().BindingRuleCreate(gatewayConsulBindingRule(), &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

func IsEnterprise() bool {
	return strings.HasSuffix(consulImage, "ent")
}

func CreateConsulNamespace(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	if IsEnterprise() {
		log.Print("Creating Consul Namespace")
		namespace := envconf.RandomName("test", 16)

		consulEnvironment := ctx.Value(consulTestContextKey)
		if consulEnvironment == nil {
			return ctx, nil
		}
		env := consulEnvironment.(*consulTestEnvironment)
		_, _, err := env.consulClient.Namespaces().Create(&api.Namespace{
			Name: namespace,
		}, &api.WriteOptions{
			Token: env.token,
		})
		if err != nil {
			return nil, err
		}
		env.namespace = namespace
	}
	return ctx, nil
}

func gatewayConsulAuthMethod(name, token string, k8sConfig *rest.Config) *api.ACLAuthMethod {
	return &api.ACLAuthMethod{
		Type: "kubernetes",
		Name: "consul-api-gateway",
		Config: map[string]interface{}{
			"Host":              fmt.Sprintf("https://%s-control-plane:6443", name),
			"CACert":            string(k8sConfig.CAData),
			"ServiceAccountJWT": token,
		},
	}
}

func gatewayConsulBindingRule() *api.ACLBindingRule {
	return &api.ACLBindingRule{
		AuthMethod: "consul-api-gateway",
		BindType:   api.BindingRuleBindTypeRole,
		BindName:   "consul-api-gateway",
		Selector:   `serviceaccount.name=="consul-api-gateway"`,
	}
}

func gatewayConsulRole(policyID string) *api.ACLRole {
	return &api.ACLRole{
		Name: "consul-api-gateway",
		Policies: []*api.ACLLink{
			{
				ID:   policyID,
				Name: "consul-api-gateway",
			},
		},
	}
}

func adminPolicy() *api.ACLPolicy {
	if IsEnterprise() {
		return &api.ACLPolicy{
			Name: "consul-api-gateway",
			Rules: `
	namespace_prefix "" {
		acl = "write"
		policy = "write"
		service_prefix "" { policy = "write" }
		session_prefix "" { policy = "write" }
		node_prefix "" { policy = "read" }
	}
	event_prefix "" { policy = "write" }
	agent_prefix "" { policy = "write" }
	query_prefix "" { policy = "write" }
	operator = "write"
	keyring = "write"
	`,
		}
	}
	return &api.ACLPolicy{
		Name: "consul-api-gateway",
		Rules: `
	node_prefix "" { policy = "read" }
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

func loadLicense(path string) (string, error) {
	// attempt to load the license from the env var holding a
	// signed license
	if license := os.Getenv(envvarConsulEnterpriseLicense); license != "" {
		return license, nil
	}

	// override the license path using the env var
	if licensePath := os.Getenv(envvarConsulEnterpriseLicensePath); licensePath != "" {
		path = licensePath
	}

	// read the license from the path
	if path != "" {
		license, err := ioutil.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("error reading from license file %q: %w", path, err)
		}
		return string(license), nil
	}

	return "", nil
}
