//+build e2e

package server

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/testing/e2e"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	testenv env.Environment
)

func TestMain(m *testing.M) {
	testenv = env.New()

	testenv.Setup(
		e2e.SetUpStack,
	)

	testenv.Finish(
		e2e.TearDownStack,
	)

	testenv.Run(m)
}

func TestGatewayBasic(t *testing.T) {
	feature := features.New("gateway admission").
		Assess("basic admission and status updates", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			gatewayNamespace := gateway.Namespace(namespace)
			resources := cfg.Client().Resources(namespace)
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			gcc := &apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					Name:      configName,
					Namespace: namespace,
				},
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ImageSpec: apigwv1alpha1.ImageSpec{
						ConsulAPIGateway: e2e.DockerImage(ctx),
					},
					ConsulSpec: apigwv1alpha1.ConsulSpec{
						Address: "host.docker.internal", // we're working trough kind
						Scheme:  "https",
						PortSpec: apigwv1alpha1.PortSpec{
							GRPC: e2e.ConsulGRPCPort(ctx),
							HTTP: e2e.ConsulHTTPPort(ctx),
						},
						AuthSpec: apigwv1alpha1.AuthSpec{
							Method:  "consul-api-gateway",
							Account: "consul-api-gateway",
						},
					},
				},
			}
			err := resources.Create(ctx, gcc)
			require.NoError(t, err)

			gc := &gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					Name:      className,
					Namespace: namespace,
				},
				Spec: gateway.GatewayClassSpec{
					ControllerName: k8s.ControllerName,
					ParametersRef: &gateway.ParametersReference{
						Group:     apigwv1alpha1.Group,
						Kind:      apigwv1alpha1.GatewayClassConfigKind,
						Name:      configName,
						Namespace: &gatewayNamespace,
					},
				},
			}
			err = resources.Create(ctx, gc)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				created := &gateway.GatewayClass{}
				if err := resources.Get(ctx, className, namespace, created); err != nil {
					return false
				}

				for _, condition := range created.Status.Conditions {
					if condition.Type == "Accepted" ||
						condition.Status == "True" {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second, "gatewayclass not accepted in the allotted time")

			gw := &gateway.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gateway.GatewaySpec{
					GatewayClassName: gateway.ObjectName(className),
					Listeners: []gateway.Listener{{
						Name:     "https",
						Port:     gateway.PortNumber(443),
						Protocol: gateway.HTTPSProtocolType,
						TLS: &gateway.GatewayTLSConfig{
							CertificateRefs: []*gateway.SecretObjectReference{{
								Name:      "consul-server-cert",
								Namespace: &gatewayNamespace,
							}},
						},
					}},
				},
			}
			err = resources.Create(ctx, gw)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				deployment := &apps.Deployment{}
				if err := resources.Get(ctx, gatewayName, namespace, deployment); err != nil {
					return false
				}
				return true
			}, 30*time.Second, 1*time.Second, "no deployment found in the allotted time")

			require.Eventually(t, func() bool {
				updated := &gateway.Gateway{}
				if err := resources.Get(ctx, gatewayName, namespace, updated); err != nil {
					return false
				}
				for _, condition := range updated.Status.Conditions {
					if condition.Type == "Accepted" ||
						condition.Status == "True" {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second, "no gateway found in the allotted time")

			// check for the service being registered
			client := e2e.ConsulClient(ctx)
			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", nil)
				if err != nil {
					return false
				}
				if len(services) != 1 {
					return false
				}
				service := services[0]
				status := service.Checks.AggregatedStatus()
				return status == "passing"
			}, 30*time.Second, 1*time.Second, "no healthy consul service found in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestServiceListeners(t *testing.T) {
	feature := features.New("service updates").
		Assess("port exposure for updated listeners", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			gatewayNamespace := gateway.Namespace(namespace)
			resources := cfg.Client().Resources(namespace)
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			serviceType := core.ServiceTypeNodePort
			gcc := &apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					Name:      configName,
					Namespace: namespace,
				},
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ImageSpec: apigwv1alpha1.ImageSpec{
						ConsulAPIGateway: e2e.DockerImage(ctx),
					},
					ServiceType: &serviceType,
					ConsulSpec: apigwv1alpha1.ConsulSpec{
						Address: "host.docker.internal", // we're working trough kind
						Scheme:  "https",
						PortSpec: apigwv1alpha1.PortSpec{
							GRPC: e2e.ConsulGRPCPort(ctx),
							HTTP: e2e.ConsulHTTPPort(ctx),
						},
						AuthSpec: apigwv1alpha1.AuthSpec{
							Method:  "consul-api-gateway",
							Account: "consul-api-gateway",
						},
					},
				},
			}
			err := resources.Create(ctx, gcc)
			require.NoError(t, err)

			gc := &gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					Name:      className,
					Namespace: namespace,
				},
				Spec: gateway.GatewayClassSpec{
					ControllerName: k8s.ControllerName,
					ParametersRef: &gateway.ParametersReference{
						Group:     apigwv1alpha1.Group,
						Kind:      apigwv1alpha1.GatewayClassConfigKind,
						Name:      configName,
						Namespace: &gatewayNamespace,
					},
				},
			}
			err = resources.Create(ctx, gc)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				created := &gateway.GatewayClass{}
				if err := resources.Get(ctx, className, namespace, created); err != nil {
					return false
				}

				for _, condition := range created.Status.Conditions {
					if condition.Type == "Accepted" ||
						condition.Status == "True" {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second, "gatewayclass not accepted in the allotted time")

			gw := &gateway.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gateway.GatewaySpec{
					GatewayClassName: gateway.ObjectName(className),
					Listeners: []gateway.Listener{{
						Name:     "https",
						Port:     gateway.PortNumber(443),
						Protocol: gateway.HTTPSProtocolType,
						TLS: &gateway.GatewayTLSConfig{
							CertificateRefs: []*gateway.SecretObjectReference{{
								Name:      "consul-server-cert",
								Namespace: &gatewayNamespace,
							}},
						},
					}},
				},
			}
			err = resources.Create(ctx, gw)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				service := &core.Service{}
				if err := resources.Get(ctx, gatewayName, namespace, service); err != nil {
					return false
				}
				if len(service.Spec.Ports) != 1 {
					return false
				}
				port := service.Spec.Ports[0]
				return port.Port == 443
			}, 30*time.Second, 1*time.Second, "no service found in the allotted time")

			// update the class config to ensure our config snapshot works
			err = resources.Get(ctx, gcc.Name, gcc.Namespace, gcc)
			require.NoError(t, err)
			serviceType = core.ServiceTypeLoadBalancer
			gcc.Spec.ServiceType = &serviceType
			err = resources.Update(ctx, gcc)
			require.NoError(t, err)

			err = resources.Get(ctx, gatewayName, namespace, gw)
			require.NoError(t, err)
			gw.Spec.Listeners[0].Port = 444
			err = resources.Update(ctx, gw)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				service := &core.Service{}
				if err := resources.Get(ctx, gatewayName, namespace, service); err != nil {
					return false
				}
				if len(service.Spec.Ports) != 1 {
					return false
				}
				require.Equal(t, core.ServiceTypeNodePort, service.Spec.Type)
				port := service.Spec.Ports[0]
				return port.Port == 444
			}, 30*time.Second, 1*time.Second, "service not updated in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}
