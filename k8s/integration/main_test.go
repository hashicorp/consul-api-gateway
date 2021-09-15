package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/polar/internal/testing/integration"
	"github.com/hashicorp/polar/k8s"
	polarv1alpha1 "github.com/hashicorp/polar/k8s/apis/v1alpha1"
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
		integration.SetUpStack,
	)

	testenv.Finish(
		integration.TearDownStack,
	)

	testenv.Run(m)
}

func TestGatewayClass(t *testing.T) {
	feature := features.New("gateway class admission").
		Assess("admission for valid class config", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := integration.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			gcc := &polarv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					Name:      configName,
					Namespace: namespace,
				},
				Spec: polarv1alpha1.GatewayClassConfigSpec{
					ImageSpec: polarv1alpha1.ImageSpec{
						Polar: integration.DockerImage(ctx),
					},
					ConsulSpec: polarv1alpha1.ConsulSpec{
						Address: "host.docker.internal", // we're working trough kind
						Scheme:  "https",
						PortSpec: polarv1alpha1.PortSpec{
							GRPC: integration.ConsulGRPCPort(ctx),
							HTTP: integration.ConsulHTTPPort(ctx),
						},
						AuthSpec: polarv1alpha1.AuthSpec{
							Method:  "polar",
							Account: "polar",
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
					Controller: k8s.ControllerName,
					ParametersRef: &gateway.ParametersReference{
						Group:     polarv1alpha1.Group,
						Kind:      polarv1alpha1.GatewayClassConfigKind,
						Name:      configName,
						Namespace: &namespace,
					},
				},
			}
			err = resources.Create(ctx, gc)
			require.NoError(t, err)

			err = backoff.Retry(func() error {
				created := &gateway.GatewayClass{}
				err = resources.Get(ctx, className, namespace, created)
				require.NoError(t, err)

				if len(created.Status.Conditions) == 0 {
					return errors.New("invalid status conditions")
				}
				condition := created.Status.Conditions[0]
				if condition.Type != "Admitted" ||
					condition.Status != "True" {
					return errors.New("gateway class not admitted")
				}
				return nil
			}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 20), ctx))
			require.NoError(t, err)

			gw := &gateway.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gateway.GatewaySpec{
					GatewayClassName: className,
					Listeners: []gateway.Listener{{
						Name:     "https",
						Port:     gateway.PortNumber(443),
						Protocol: gateway.HTTPSProtocolType,
						TLS: &gateway.GatewayTLSConfig{
							CertificateRef: &gateway.ObjectReference{
								Name:      "consul-server-cert",
								Namespace: &namespace,
							},
						},
					}},
				},
			}
			err = resources.Create(ctx, gw)
			require.NoError(t, err)

			// check for the deployment
			err = backoff.Retry(func() error {
				deployment := &apps.Deployment{}
				return resources.Get(ctx, gatewayName, namespace, deployment)
			}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 20), ctx))
			require.NoError(t, err)

			// check for the service being registered
			client := integration.ConsulClient(ctx)
			var status string
			err = backoff.Retry(func() error {
				services, _, err := client.Catalog().Service(gatewayName, "", nil)
				if err != nil {
					fmt.Println(err)
					return err
				}
				if len(services) != 1 {
					return errors.New("service not found")
				}
				service := services[0]
				status = service.Checks.AggregatedStatus()
				if status != "passing" {
					return errors.New("service unhealthy")
				}
				return nil
			}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 20), ctx))
			require.NoError(t, err)

			return ctx
		})

	testenv.Test(t, feature.Feature())
}
