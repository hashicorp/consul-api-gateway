//+build e2e

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/consul-api-gateway/internal/testing/e2e"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

var (
	testenv   env.Environment
	hostRoute string
)

func init() {
	hostRoute = os.Getenv("DOCKER_HOST_ROUTE")
	if hostRoute == "" {
		hostRoute = "host.docker.internal"
	}
}

func TestMain(m *testing.M) {
	testenv = env.New()

	testenv.Setup(
		e2e.SetUpStack(hostRoute),
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

			gatewayName := envconf.RandomName("gw", 16)
			_, gc := createGatewayClass(ctx, t, cfg)

			require.Eventually(t, func() bool {
				created := &gateway.GatewayClass{}
				if err := resources.Get(ctx, gc.Name, "", created); err != nil {
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
					GatewayClassName: gateway.ObjectName(gc.Name),
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
			err := resources.Create(ctx, gw)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				deployment := &apps.Deployment{}
				if err := resources.Get(ctx, gatewayName, namespace, deployment); err != nil {
					return false
				}
				return true
			}, 30*time.Second, 1*time.Second, "no deployment found in the allotted time")

			created := &gateway.Gateway{}
			require.Eventually(t, func() bool {
				if err := resources.Get(ctx, gatewayName, namespace, created); err != nil {
					return false
				}
				for _, condition := range created.Status.Conditions {
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

			err = resources.Delete(ctx, created)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", nil)
				if err != nil {
					return false
				}
				return len(services) == 0
			}, 30*time.Second, 1*time.Second, "consul service not deregistered in the allotted time")

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

			gatewayName := envconf.RandomName("gw", 16)
			gcc, gc := createGatewayClass(ctx, t, cfg)

			gw := &gateway.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gateway.GatewaySpec{
					GatewayClassName: gateway.ObjectName(gc.Name),
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
			err := resources.Create(ctx, gw)
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
			serviceType := core.ServiceTypeLoadBalancer
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

func TestMeshService(t *testing.T) {
	feature := features.New("mesh service routing").
		Assess("basic routing", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceThree, err := e2e.DeployMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceFour, err := e2e.DeployMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceFive, err := e2e.DeployMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespace(ctx)
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)
			routeThreeName := envconf.RandomName("route", 16)
			routeFourName := envconf.RandomName("route", 16)

			gatewayNamespace := gateway.Namespace(namespace)
			resources := cfg.Client().Resources(namespace)

			gcc := &apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					Name: configName,
				},
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ImageSpec: apigwv1alpha1.ImageSpec{
						ConsulAPIGateway: e2e.DockerImage(ctx),
					},
					UseHostPorts: true,
					LogLevel:     "trace",
					ConsulSpec: apigwv1alpha1.ConsulSpec{
						Address: hostRoute,
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
			err = resources.Create(ctx, gcc)
			require.NoError(t, err)

			gc := &gateway.GatewayClass{
				ObjectMeta: meta.ObjectMeta{
					Name: className,
				},
				Spec: gateway.GatewayClassSpec{
					ControllerName: k8s.ControllerName,
					ParametersRef: &gateway.ParametersReference{
						Group: apigwv1alpha1.Group,
						Kind:  apigwv1alpha1.GatewayClassConfigKind,
						Name:  configName,
					},
				},
			}
			err = resources.Create(ctx, gc)
			require.NoError(t, err)

			gw := &gateway.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gateway.GatewaySpec{
					GatewayClassName: gateway.ObjectName(gc.Name),
					Listeners: []gateway.Listener{{
						Name:     "https",
						Port:     gateway.PortNumber(e2e.ExtraPort(ctx)),
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
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, gatewayReady), 30*time.Second, 1*time.Second, "no gateway found in the allotted time")

			// route 1
			port := gateway.PortNumber(serviceOne.Spec.Ports[0].Port)
			path := "/v1"
			pathMatch := gateway.PathMatchExact
			routeOne := &gateway.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeOneName,
					Namespace: namespace,
				},
				Spec: gateway.HTTPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name: gateway.ObjectName(gatewayName),
						}},
					},
					Rules: []gateway.HTTPRouteRule{{
						Matches: []gateway.HTTPRouteMatch{{
							Path: &gateway.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
						}},
						BackendRefs: []gateway.HTTPBackendRef{{
							BackendRef: gateway.BackendRef{
								BackendObjectReference: gateway.BackendObjectReference{
									Name: gateway.ObjectName(serviceOne.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, routeOne)
			require.NoError(t, err)

			// route 2
			port = gateway.PortNumber(serviceTwo.Spec.Ports[0].Port)
			path = "/v2"
			route := &gateway.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeTwoName,
					Namespace: namespace,
				},
				Spec: gateway.HTTPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name: gateway.ObjectName(gatewayName),
						}},
					},
					Rules: []gateway.HTTPRouteRule{{
						Matches: []gateway.HTTPRouteMatch{{
							Path: &gateway.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
						}},
						BackendRefs: []gateway.HTTPBackendRef{{
							BackendRef: gateway.BackendRef{
								BackendObjectReference: gateway.BackendObjectReference{
									Name: gateway.ObjectName(serviceTwo.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			// route 3
			port = gateway.PortNumber(serviceThree.Spec.Ports[0].Port)
			path = "/v3"
			headerMatch := gateway.HeaderMatchExact
			route = &gateway.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeThreeName,
					Namespace: namespace,
				},
				Spec: gateway.HTTPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name: gateway.ObjectName(gatewayName),
						}},
					},
					Hostnames: []gateway.Hostname{"test.host"},
					Rules: []gateway.HTTPRouteRule{{
						Matches: []gateway.HTTPRouteMatch{{
							Path: &gateway.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
							Headers: []gateway.HTTPHeaderMatch{{
								Type:  &headerMatch,
								Name:  gateway.HTTPHeaderName("x-v3"),
								Value: "v3",
							}},
						}},
						BackendRefs: []gateway.HTTPBackendRef{{
							BackendRef: gateway.BackendRef{
								BackendObjectReference: gateway.BackendObjectReference{
									Name: gateway.ObjectName(serviceThree.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			// route 4 - fallback
			portFour := gateway.PortNumber(serviceFour.Spec.Ports[0].Port)
			portFive := gateway.PortNumber(serviceFive.Spec.Ports[0].Port)
			route = &gateway.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeFourName,
					Namespace: namespace,
				},
				Spec: gateway.HTTPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name: gateway.ObjectName(gatewayName),
						}},
					},
					Rules: []gateway.HTTPRouteRule{{
						BackendRefs: []gateway.HTTPBackendRef{{
							BackendRef: gateway.BackendRef{
								BackendObjectReference: gateway.BackendObjectReference{
									Name: gateway.ObjectName(serviceFour.Name),
									Port: &portFour,
								},
							},
						}, {
							BackendRef: gateway.BackendRef{
								BackendObjectReference: gateway.BackendObjectReference{
									Name: gateway.ObjectName(serviceFive.Name),
									Port: &portFive,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			checkPort := e2e.ExtraPort(ctx)
			checkRoute(t, checkPort, "/v1", serviceOne.Name, nil, "service one not routable in allotted time")
			checkRoute(t, checkPort, "/v2", serviceTwo.Name, nil, "service two not routable in allotted time")
			checkRoute(t, checkPort, "/v3", serviceThree.Name, map[string]string{
				"x-v3": "v3",
				"Host": "test.host",
			}, "service three not routable in allotted time")
			checkRoute(t, checkPort, "/v3", serviceFour.Name, nil, "service four not routable in allotted time")
			checkRoute(t, checkPort, "/v3", serviceFive.Name, nil, "service five not routable in allotted time")

			err = resources.Delete(ctx, routeOne)
			require.NoError(t, err)

			checkRoute(t, checkPort, "/v1", serviceFour.Name, nil, "after route deletion service four not routable in allotted time")
			checkRoute(t, checkPort, "/v1", serviceFive.Name, nil, "after route deletion service five not routable in allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, gatewayInSync), 30*time.Second, 1*time.Second, "gateway not synced in the allotted time")
			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func gatewayStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gateway.Gateway{}
		if err := resources.Get(ctx, gatewayName, namespace, updated); err != nil {
			return false
		}
		return checkFn(updated.Status.Conditions)
	}
}

func gatewayReady(conditions []meta.Condition) bool {
	for _, condition := range conditions {
		if condition.Type == "Accepted" ||
			condition.Status == "True" {
			return true
		}
	}
	return false
}

func gatewayInSync(conditions []meta.Condition) bool {
	for _, condition := range conditions {
		if condition.Type == "InSync" ||
			condition.Status == "True" {
			return true
		}
	}
	return false
}

func createGatewayClass(ctx context.Context, t *testing.T, cfg *envconf.Config) (*apigwv1alpha1.GatewayClassConfig, *gateway.GatewayClass) {
	t.Helper()

	namespace := e2e.Namespace(ctx)
	configName := envconf.RandomName("gcc", 16)
	className := envconf.RandomName("gc", 16)
	serviceType := core.ServiceTypeNodePort

	resources := cfg.Client().Resources(namespace)

	gcc := &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: configName,
		},
		Spec: apigwv1alpha1.GatewayClassConfigSpec{
			ImageSpec: apigwv1alpha1.ImageSpec{
				ConsulAPIGateway: e2e.DockerImage(ctx),
			},
			ServiceType: &serviceType,
			ConsulSpec: apigwv1alpha1.ConsulSpec{
				Address: hostRoute,
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
			Name: className,
		},
		Spec: gateway.GatewayClassSpec{
			ControllerName: k8s.ControllerName,
			ParametersRef: &gateway.ParametersReference{
				Group: apigwv1alpha1.Group,
				Kind:  apigwv1alpha1.GatewayClassConfigKind,
				Name:  configName,
			},
		},
	}
	err = resources.Create(ctx, gc)
	require.NoError(t, err)

	return gcc, gc
}

func checkRoute(t *testing.T, port int, path, expected string, headers map[string]string, message string) {
	t.Helper()

	require.Eventually(t, func() bool {
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
		req, err := http.NewRequest("GET", fmt.Sprintf("https://localhost:%d%s", port, path), nil)
		if err != nil {
			return false
		}

		for k, v := range headers {
			req.Header.Set(k, v)

			if k == "Host" {
				req.Host = v
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}

		if resp.StatusCode != http.StatusOK {
			return false
		}

		return strings.HasPrefix(string(data), expected)
	}, 30*time.Second, 1*time.Second, message)
}
