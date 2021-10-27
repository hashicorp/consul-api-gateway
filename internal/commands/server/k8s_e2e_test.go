// +build e2e

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

func TestGatewayClass(t *testing.T) {
	feature := features.New("gateway class admission").
		Assess("admission for valid class config", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
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
				err = resources.Get(ctx, className, namespace, created)
				require.NoError(t, err)

				for _, condition := range created.Status.Conditions {
					if condition.Type == "Accepted" ||
						condition.Status == "True" {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second, "gatewayclass not accepted in the alotted time")

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
			}, 30*time.Second, 1*time.Second, "no deployment found in the alotted time")

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
			}, 30*time.Second, 1*time.Second, "no deployment found in the alotted time")

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
			}, 30*time.Second, 1*time.Second, "no healthy consul service found in the alotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}
