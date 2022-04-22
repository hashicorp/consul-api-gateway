//go:build e2e
// +build e2e

package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	"github.com/hashicorp/consul/api"
)

var (
	testenv       env.Environment
	hostRoute     string
	checkTimeout  = 1 * time.Minute
	checkInterval = 1 * time.Second
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

func TestGatewayWithClassConfigChange(t *testing.T) {
	feature := features.New("gateway admission").
		Assess("gateway behavior on class config change", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespaces(ctx)[0]
			resources := cfg.Client().Resources(namespace)

			// Create a GatewayClassConfig
			firstConfig, gc := createGatewayClass(ctx, t, cfg)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), 30*time.Second, checkInterval, "gatewayclass not accepted in the allotted time")

			oldUseHostPorts := firstConfig.Spec.UseHostPorts

			// Create a Gateway and wait for it to be ready
			firstGatewayName := envconf.RandomName("gw", 16)
			firstGateway := createGateway(ctx, t, cfg, firstGatewayName, gc, 443, nil)
			require.Eventually(t, func() bool {
				err := resources.Get(ctx, firstGatewayName, namespace, firstGateway)
				return err == nil && conditionAccepted(firstGateway.Status.Conditions)
			}, 60*time.Second, checkInterval, "no gateway found in the allotted time")
			require.Eventually(t, gatewayStatusCheck(ctx, resources, firstGatewayName, namespace, conditionReady), 30*time.Second, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(t, firstGateway, firstConfig)

			// Modify GatewayClassConfig used for Gateway
			secondConfig := &apigwv1alpha1.GatewayClassConfig{}
			require.NoError(t, resources.Get(ctx, firstConfig.Name, namespace, secondConfig))

			newUseHostPorts := !oldUseHostPorts
			secondConfig.Spec.UseHostPorts = newUseHostPorts
			require.NoError(t, resources.Update(ctx, secondConfig))

			// Create a second Gateway and wait for it to be ready
			secondGatewayName := envconf.RandomName("gw", 16)
			secondGateway := createGateway(ctx, t, cfg, secondGatewayName, gc, 443, nil)
			require.Eventually(t, func() bool {
				err := resources.Get(ctx, secondGatewayName, namespace, secondGateway)
				return err == nil && conditionAccepted(secondGateway.Status.Conditions)
			}, 30*time.Second, checkInterval, "no gateway found in the allotted time")
			require.Eventually(t, gatewayStatusCheck(ctx, resources, secondGatewayName, namespace, conditionReady), 30*time.Second, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(t, secondGateway, secondConfig)

			// Verify that 1st Gateway retains initial GatewayClassConfig and 2nd Gateway retains updated GatewayClassConfig
			checkGatewayConfigAnnotation(t, firstGateway, firstConfig)
			checkGatewayConfigAnnotation(t, secondGateway, secondConfig)

			assert.NoError(t, resources.Delete(ctx, firstGateway))
			assert.NoError(t, resources.Delete(ctx, secondGateway))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayBasic(t *testing.T) {
	feature := features.New("gateway admission").
		Assess("basic admission and status updates", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespaces(ctx)[0]
			resources := cfg.Client().Resources(namespace)

			gatewayName := envconf.RandomName("gw", 16)
			gcc, gc := createGatewayClass(ctx, t, cfg)

			require.Eventually(t, func() bool {
				created := &gateway.GatewayClass{}
				err := resources.Get(ctx, gc.Name, "", created)
				return err == nil && conditionAccepted(created.Status.Conditions)
			}, checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			_ = createGateway(ctx, t, cfg, gatewayName, gc, 443, nil)

			require.Eventually(t, func() bool {
				err := resources.Get(ctx, gatewayName, namespace, &apps.Deployment{})
				return err == nil
			}, checkTimeout, checkInterval, "no deployment found in the allotted time")

			created := &gateway.Gateway{}
			require.Eventually(t, func() bool {
				err := resources.Get(ctx, gatewayName, namespace, created)
				return err == nil && conditionAccepted(created.Status.Conditions)
			}, checkTimeout, checkInterval, "no gateway found in the allotted time")

			checkGatewayConfigAnnotation(t, created, gcc)

			// check for the service being registered
			client := e2e.ConsulClient(ctx)
			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", &api.QueryOptions{
					Namespace: e2e.ConsulNamespace(ctx),
				})
				if err != nil {
					return false
				}
				if len(services) != 1 {
					return false
				}
				service := services[0]
				status := service.Checks.AggregatedStatus()
				return status == "passing"
			}, checkTimeout, checkInterval, "no healthy consul service found in the allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			err := resources.Delete(ctx, created)
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", &api.QueryOptions{
					Namespace: e2e.ConsulNamespace(ctx),
				})
				if err != nil {
					return false
				}
				return len(services) == 0
			}, checkTimeout, checkInterval, "consul service not deregistered in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestServiceListeners(t *testing.T) {
	feature := features.New("service updates").
		Assess("port exposure for updated listeners", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespaces(ctx)[0]
			resources := cfg.Client().Resources(namespace)

			gatewayName := envconf.RandomName("gw", 16)
			_, gc := createGatewayClass(ctx, t, cfg)

			gw := createGateway(ctx, t, cfg, gatewayName, gc, 443, nil)

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
			}, checkTimeout, checkInterval, "no service found in the allotted time")

			err := resources.Get(ctx, gatewayName, namespace, gw)
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
			}, checkTimeout, checkInterval, "service not updated in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestHTTPRouteFlattening(t *testing.T) {
	feature := features.New("http service route flattening").
		Assess("basic routing", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespaces(ctx)[0]
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)

			prefixMatch := gateway.PathMatchPathPrefix
			headerMatch := gateway.HeaderMatchExact

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

			checkPort := e2e.HTTPFlattenedPort(ctx)
			gw := createGateway(ctx, t, cfg, gatewayName, gc, gateway.PortNumber(checkPort), nil)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			port := gateway.PortNumber(serviceOne.Spec.Ports[0].Port)
			path := "/"
			route := &gateway.HTTPRoute{
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
					Hostnames: []gateway.Hostname{"test.foo", "test.example"},
					Rules: []gateway.HTTPRouteRule{{
						Matches: []gateway.HTTPRouteMatch{{
							Path: &gateway.HTTPPathMatch{
								Type:  &prefixMatch,
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
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			port = gateway.PortNumber(serviceTwo.Spec.Ports[0].Port)
			path = "/v2"
			route = &gateway.HTTPRoute{
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
					Hostnames: []gateway.Hostname{"test.foo"},
					Rules: []gateway.HTTPRouteRule{{
						Matches: []gateway.HTTPRouteMatch{{
							Path: &gateway.HTTPPathMatch{
								Type:  &prefixMatch,
								Value: &path,
							},
						}, {
							Headers: []gateway.HTTPHeaderMatch{{
								Type:  &headerMatch,
								Name:  gateway.HTTPHeaderName("x-v2"),
								Value: "v2",
							}},
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

			checkRoute(t, checkPort, "/v2/test", serviceTwo.Name, map[string]string{
				"Host": "test.foo",
			}, "service two not routable in allotted time")
			checkRoute(t, checkPort, "/", serviceOne.Name, map[string]string{
				"Host": "test.foo",
			}, "service one not routable in allotted time")
			checkRoute(t, checkPort, "/", serviceTwo.Name, map[string]string{
				"Host": "test.foo",
				"x-v2": "v2",
			}, "service two with headers is not routable in allotted time")

			err = resources.Delete(ctx, gw)
			require.NoError(t, err)

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestHTTPMeshService(t *testing.T) {
	feature := features.New("mesh service routing").
		Assess("basic routing", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)
			// register this service in a different consul namespace
			serviceThree, err := e2e.DeployHTTPMeshService(ctx, cfg, e2e.ConsulNamespace(ctx))
			require.NoError(t, err)
			serviceFour, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceFive, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespaces(ctx)[0]
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)
			routeThreeName := envconf.RandomName("route", 16)

			resources := cfg.Client().Resources(namespace)

			gcc := &apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					Name: configName,
				},
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ImageSpec: apigwv1alpha1.ImageSpec{
						ConsulAPIGateway: e2e.DockerImage(ctx),
					},
					ServiceType:  serviceType(core.ServiceTypeNodePort),
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

			gw := createGateway(ctx, t, cfg, gatewayName, gc, gateway.PortNumber(e2e.HTTPPort(ctx)), nil)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

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
			portFour := gateway.PortNumber(serviceFour.Spec.Ports[0].Port)
			portFive := gateway.PortNumber(serviceFive.Spec.Ports[0].Port)
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
					}, {
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

			checkPort := e2e.HTTPPort(ctx)
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

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionInSync), checkTimeout, checkInterval, "gateway not synced in the allotted time")

			client := e2e.ConsulClient(ctx)
			require.Eventually(t, func() bool {
				entry, _, err := client.ConfigEntries().Get(api.IngressGateway, gatewayName, &api.QueryOptions{
					Namespace: e2e.ConsulNamespace(ctx),
				})
				if err != nil {
					return false
				}
				return entry != nil
			}, checkTimeout, checkInterval, "no consul config entry found")

			err = resources.Delete(ctx, gw)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				_, _, err := client.ConfigEntries().Get(api.IngressGateway, gatewayName, &api.QueryOptions{
					Namespace: e2e.ConsulNamespace(ctx),
				})
				if err == nil {
					return false
				}
				return strings.Contains(err.Error(), "Unexpected response code: 404")
			}, checkTimeout, checkInterval, "consul config entry not cleaned up")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestTCPMeshService(t *testing.T) {
	feature := features.New("mesh service tcp routing").
		Assess("basic routing", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceThree, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)
			// register this in the proper Consul namespace so that we can map and resolve it as a MeshService
			serviceFour, err := e2e.DeployTCPMeshService(ctx, cfg, e2e.ConsulNamespace(ctx))
			require.NoError(t, err)

			namespace := e2e.Namespaces(ctx)[0]
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)

			resources := cfg.Client().Resources(namespace)

			// create a MeshService to route to service four
			meshServiceName := envconf.RandomName("meshsvc", 16)
			require.NoError(t, resources.Create(ctx, &apigwv1alpha1.MeshService{
				ObjectMeta: meta.ObjectMeta{
					Name:      meshServiceName,
					Namespace: namespace,
				},
				Spec: apigwv1alpha1.MeshServiceSpec{
					Name: serviceFour.Name,
				},
			}))

			gcc := &apigwv1alpha1.GatewayClassConfig{
				ObjectMeta: meta.ObjectMeta{
					Name: configName,
				},
				Spec: apigwv1alpha1.GatewayClassConfigSpec{
					ImageSpec: apigwv1alpha1.ImageSpec{
						ConsulAPIGateway: e2e.DockerImage(ctx),
					},
					ServiceType:  serviceType(core.ServiceTypeNodePort),
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
						Name:     "tcp",
						Port:     gateway.PortNumber(e2e.TCPPort(ctx)),
						Protocol: gateway.TCPProtocolType,
					}},
				},
			}
			err = resources.Create(ctx, gw)
			require.NoError(t, err)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// route 1
			portOne := gateway.PortNumber(serviceOne.Spec.Ports[0].Port)
			portTwo := gateway.PortNumber(serviceTwo.Spec.Ports[0].Port)
			portThree := gateway.PortNumber(serviceThree.Spec.Ports[0].Port)
			routeOne := &gateway.TCPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeOneName,
					Namespace: namespace,
				},
				Spec: gateway.TCPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name: gateway.ObjectName(gatewayName),
						}},
					},
					Rules: []gateway.TCPRouteRule{{
						BackendRefs: []gateway.BackendRef{{
							BackendObjectReference: gateway.BackendObjectReference{
								Name: gateway.ObjectName(serviceOne.Name),
								Port: &portOne,
							},
						}, {
							BackendObjectReference: gateway.BackendObjectReference{
								Name: gateway.ObjectName(serviceTwo.Name),
								Port: &portTwo,
							},
						}},
					}, {
						BackendRefs: []gateway.BackendRef{{
							BackendObjectReference: gateway.BackendObjectReference{
								Name: gateway.ObjectName(serviceThree.Name),
								Port: &portThree,
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, routeOne)
			require.NoError(t, err)

			require.Eventually(t, tcpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				routeOneName,
				namespace,
				createConditionCheckWithReason("ResolvedRefs", "False", "Errors"),
			), checkTimeout, checkInterval, "route status not set in allotted time")

			// route 2
			meshServiceGroup := gateway.Group(apigwv1alpha1.Group)
			meshServiceKind := gateway.Kind(apigwv1alpha1.MeshServiceKind)
			// this routes to service four
			route := &gateway.TCPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeTwoName,
					Namespace: namespace,
				},
				Spec: gateway.TCPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name: gateway.ObjectName(gatewayName),
						}},
					},
					Rules: []gateway.TCPRouteRule{{
						BackendRefs: []gateway.BackendRef{{
							BackendObjectReference: gateway.BackendObjectReference{
								Group: &meshServiceGroup,
								Kind:  &meshServiceKind,
								Name:  gateway.ObjectName(meshServiceName),
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			checkPort := e2e.TCPPort(ctx)

			// only service 4 should be routable as we don't support routes with multiple rules or backend refs for TCP
			checkTCPRoute(t, checkPort, serviceFour.Name, "service four not routable in allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionInSync), checkTimeout, checkInterval, "gateway not synced in the allotted time")
			return ctx
		}).
		Assess("tls routing", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespaces(ctx)[0]
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)
			listenerOneName := "tcp"
			listenerTwoName := "insecure"
			listenerOnePort := e2e.TCPTLSPort(ctx)
			listenerTwoPort := e2e.ExtraTCPTLSPort(ctx)

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
					ServiceType:  serviceType(core.ServiceTypeNodePort),
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
					Listeners: []gateway.Listener{
						{
							Name:     gateway.SectionName(listenerOneName),
							Port:     gateway.PortNumber(listenerOnePort),
							Protocol: gateway.TCPProtocolType,
							TLS: &gateway.GatewayTLSConfig{
								CertificateRefs: []*gateway.SecretObjectReference{{
									Name:      "consul-server-cert",
									Namespace: &gatewayNamespace,
								}},
							},
						},
						{
							Name:     gateway.SectionName(listenerTwoName),
							Port:     gateway.PortNumber(listenerTwoPort),
							Protocol: gateway.TCPProtocolType,
							TLS: &gateway.GatewayTLSConfig{
								CertificateRefs: []*gateway.SecretObjectReference{{
									Name:      "consul-server-cert",
									Namespace: &gatewayNamespace,
								}},
								Options: map[gateway.AnnotationKey]gateway.AnnotationValue{
									"api-gateway.consul.hashicorp.com/tls_min_version":   "TLSv1_1",
									"api-gateway.consul.hashicorp.com/tls_cipher_suites": "TLS_RSA_WITH_AES_128_CBC_SHA",
								},
							},
						},
					},
				},
			}
			err = resources.Create(ctx, gw)
			require.NoError(t, err)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			createTCPRoute(ctx, t, resources, namespace, gatewayName, gateway.SectionName(listenerOneName), routeOneName, serviceOne.Name, gateway.PortNumber(serviceOne.Spec.Ports[0].Port))
			createTCPRoute(ctx, t, resources, namespace, gatewayName, gateway.SectionName(listenerTwoName), routeTwoName, serviceTwo.Name, gateway.PortNumber(serviceTwo.Spec.Ports[0].Port))

			checkTCPTLSRoute(t, listenerOnePort, &tls.Config{
				InsecureSkipVerify: true,
			}, serviceOne.Name, "service not routable in allotted time")

			// Force insecure cipher suite excluded from Consul API Gateway default
			// cipher suites, but supported by Envoy defaults, limit max version to
			// TLS 1.2 to ensure cipher suite config is applicable.
			checkTCPTLSRoute(t, listenerOnePort, &tls.Config{
				InsecureSkipVerify: true,
				MaxVersion:         tls.VersionTLS12,
				CipherSuites:       []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA},
			}, "remote error: tls: handshake failure", "connection not rejected with expected error in allotted time")

			// Force TLS max version below Consul API Gateway default min version, but
			// supported by Envoy defaults
			checkTCPTLSRoute(t, listenerOnePort, &tls.Config{
				InsecureSkipVerify: true,
				MaxVersion:         tls.VersionTLS11,
			}, "remote error: tls: protocol version not supported", "connection not rejected with expected error in allotted time")

			// Service two listener overrides default config
			checkTCPTLSRoute(t, listenerTwoPort, &tls.Config{
				InsecureSkipVerify: true,
				CipherSuites:       []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA},
				MaxVersion:         tls.VersionTLS11,
			}, serviceTwo.Name, "service not routable in allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionInSync), checkTimeout, checkInterval, "gateway not synced in the allotted time")

			require.Eventually(t, listenerStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "listeners not ready in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestHTTPRouteReferencePolicyLifecycle(t *testing.T) {
	feature := features.New("http route reference policy").
		Assess("http route controller watches reference policy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespaces(ctx)[0]
			configName := envconf.RandomName("gcc", 16)
			className := envconf.RandomName("gc", 16)
			gatewayName := envconf.RandomName("gw", 16)
			routeName := envconf.RandomName("route", 16)
			routeNamespace := e2e.Namespaces(ctx)[1]
			refPolicyName := envconf.RandomName("refpolicy", 16)

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

			// FIXME: should this use a different port?
			checkPort := e2e.HTTPFlattenedPort(ctx)

			// Allow routes to bind from a different namespace for testing
			// cross-namespace ReferencePolicy enforcement
			all := gateway.NamespacesFromAll
			allowedRoutes := &gateway.AllowedRoutes{
				Namespaces: &gateway.RouteNamespaces{
					From: &all,
				},
			}

			gw := createGateway(ctx, t, cfg, gatewayName, gc, gateway.PortNumber(checkPort), allowedRoutes)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			port := gateway.PortNumber(serviceOne.Spec.Ports[0].Port)
			gwNamespace := gateway.Namespace(namespace)
			route := &gateway.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeName,
					Namespace: routeNamespace,
				},
				Spec: gateway.HTTPRouteSpec{
					CommonRouteSpec: gateway.CommonRouteSpec{
						ParentRefs: []gateway.ParentRef{{
							Name:      gateway.ObjectName(gatewayName),
							Namespace: &gwNamespace,
						}},
					},
					Hostnames: []gateway.Hostname{"test.foo"},
					Rules: []gateway.HTTPRouteRule{{
						BackendRefs: []gateway.HTTPBackendRef{{
							BackendRef: gateway.BackendRef{
								BackendObjectReference: gateway.BackendObjectReference{
									Name:      gateway.ObjectName(serviceOne.Name),
									Namespace: &gwNamespace,
									Port:      &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			// Expect that route sets
			// ResolvedRefs{ status: False, reason: RefNotPermitted }
			// due to missing ReferencePolicy for BackendRef in other namespace
			httpRouteStatusCheckRefNotPermitted := httpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				routeName,
				routeNamespace,
				createConditionCheckWithReason(
					"ResolvedRefs",
					"False",
					"RefNotPermitted",
				),
			)
			require.Eventually(t, httpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "route status not set in allotted time")

			// create ReferencePolicy allowing BackendRef
			serviceOneObjectName := gateway.ObjectName(serviceOne.Name)
			referencePolicy := &gateway.ReferencePolicy{
				ObjectMeta: meta.ObjectMeta{
					Name:      refPolicyName,
					Namespace: namespace,
				},
				Spec: gateway.ReferencePolicySpec{
					From: []gateway.ReferencePolicyFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "HTTPRoute",
						Namespace: gateway.Namespace(routeNamespace),
					}},
					To: []gateway.ReferencePolicyTo{{
						Group: "",
						Kind:  "Service",
						Name:  &serviceOneObjectName,
					}},
				},
			}
			err = resources.Create(ctx, referencePolicy)
			require.NoError(t, err)

			// Expect that route sets
			// ResolvedRefs{ status: True, reason: ResolvedRefs }
			// now that ReferencePolicy allows BackendRef in other namespace
			require.Eventually(t, httpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				routeName,
				routeNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "True"},
					{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"},
				}),
			), checkTimeout, checkInterval, "route status not set in allotted time")

			// Check that route is successfully resolved and routing traffic
			checkRoute(t, checkPort, "/", serviceOne.Name, map[string]string{
				"Host": "test.foo",
			}, "service one not routable in allotted time")

			// Delete ReferencePolicy, check for RefNotPermitted again
			err = resources.Delete(ctx, referencePolicy)
			require.NoError(t, err)
			require.Eventually(t, httpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "route status not set in allotted time")

			err = resources.Delete(ctx, gw)
			require.NoError(t, err)

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

func gatewayClassStatusCheck(ctx context.Context, resources *resources.Resources, gatewayClassName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gateway.GatewayClass{}
		if err := resources.Get(ctx, gatewayClassName, namespace, updated); err != nil {
			return false
		}

		return checkFn(updated.Status.Conditions)
	}
}

func listenerStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gateway.Gateway{}
		if err := resources.Get(ctx, gatewayName, namespace, updated); err != nil {
			return false
		}

		for _, listener := range updated.Status.Listeners {
			if ok := checkFn(listener.Conditions); !ok {
				return false
			}
		}

		return true
	}
}

func httpRouteStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, routeName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gateway.HTTPRoute{}
		if err := resources.Get(ctx, routeName, namespace, updated); err != nil {
			return false
		}
		for _, status := range updated.Status.Parents {
			if string(status.ParentRef.Name) == gatewayName {
				return checkFn(status.Conditions)
			}
		}
		return false
	}
}

func tcpRouteStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, routeName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gateway.TCPRoute{}
		if err := resources.Get(ctx, routeName, namespace, updated); err != nil {
			return false
		}
		for _, status := range updated.Status.Parents {
			if string(status.ParentRef.Name) == gatewayName {
				return checkFn(status.Conditions)
			}
		}
		return false
	}
}

func createConditionsCheck(expected []meta.Condition) func([]meta.Condition) bool {
	return func(actual []meta.Condition) bool {
		for _, eCondition := range expected {
			matched := false
			for _, aCondition := range actual {
				if aCondition.Type == eCondition.Type &&
					aCondition.Status == eCondition.Status &&
					// Match if expected condition doesn't define an expected reason
					(aCondition.Reason == eCondition.Reason || eCondition.Reason == "") {
					matched = true
					break
				}
			}

			if !matched {
				return false
			}
		}
		return true
	}
}

func createConditionCheck(conditionType string, status meta.ConditionStatus) func([]meta.Condition) bool {
	return func(conditions []meta.Condition) bool {
		for _, condition := range conditions {
			if condition.Type == conditionType &&
				condition.Status == status {
				return true
			}
		}
		return false
	}
}

func createConditionCheckWithReason(conditionType string, status meta.ConditionStatus, reason string) func([]meta.Condition) bool {
	return func(conditions []meta.Condition) bool {
		for _, condition := range conditions {
			if condition.Type == conditionType &&
				condition.Status == status &&
				condition.Reason == reason {
				return true
			}
		}
		return false
	}
}

func conditionAccepted(conditions []meta.Condition) bool {
	return createConditionCheck("Accepted", "True")(conditions)
}

func conditionReady(conditions []meta.Condition) bool {
	return createConditionCheck("Ready", "True")(conditions)
}

func conditionInSync(conditions []meta.Condition) bool {
	return createConditionCheck("InSync", "True")(conditions)
}

func createGateway(ctx context.Context, t *testing.T, cfg *envconf.Config, gatewayName string, gc *gateway.GatewayClass, listenerPort gateway.PortNumber, listenerAllowedRoutes *gateway.AllowedRoutes) *gateway.Gateway {
	t.Helper()

	namespace := e2e.Namespaces(ctx)[0]
	gatewayNamespace := gateway.Namespace(namespace)

	resources := cfg.Client().Resources(namespace)

	gw := &gateway.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      gatewayName,
			Namespace: namespace,
		},
		Spec: gateway.GatewaySpec{
			GatewayClassName: gateway.ObjectName(gc.Name),
			Listeners: []gateway.Listener{{
				Name:     "https",
				Port:     listenerPort,
				Protocol: gateway.HTTPSProtocolType,
				TLS: &gateway.GatewayTLSConfig{
					CertificateRefs: []*gateway.SecretObjectReference{{
						Name:      "consul-server-cert",
						Namespace: &gatewayNamespace,
					}},
				},
				AllowedRoutes: listenerAllowedRoutes,
			}},
		},
	}

	err := resources.Create(ctx, gw)
	require.NoError(t, err)

	return gw
}

func createGatewayClass(ctx context.Context, t *testing.T, cfg *envconf.Config) (*apigwv1alpha1.GatewayClassConfig, *gateway.GatewayClass) {
	t.Helper()

	namespace := e2e.Namespaces(ctx)[0]
	configName := envconf.RandomName("gcc", 16)
	className := envconf.RandomName("gc", 16)

	resources := cfg.Client().Resources(namespace)

	gcc := &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name: configName,
		},
		Spec: apigwv1alpha1.GatewayClassConfigSpec{
			ImageSpec: apigwv1alpha1.ImageSpec{
				ConsulAPIGateway: e2e.DockerImage(ctx),
			},
			ServiceType: serviceType(core.ServiceTypeNodePort),
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

func createTCPRoute(ctx context.Context, t *testing.T, resources *resources.Resources, namespace string, gatewayName string, listenerName gateway.SectionName, routeName string, serviceName string, port gateway.PortNumber) {
	t.Helper()

	route := &gateway.TCPRoute{
		ObjectMeta: meta.ObjectMeta{
			Name:      routeName,
			Namespace: namespace,
		},
		Spec: gateway.TCPRouteSpec{
			CommonRouteSpec: gateway.CommonRouteSpec{
				ParentRefs: []gateway.ParentRef{{
					Name:        gateway.ObjectName(gatewayName),
					SectionName: &listenerName,
				}},
			},
			Rules: []gateway.TCPRouteRule{{
				BackendRefs: []gateway.BackendRef{{
					BackendObjectReference: gateway.BackendObjectReference{
						Name: gateway.ObjectName(serviceName),
						Port: &port,
					},
				}},
			}},
		},
	}

	err := resources.Create(ctx, route)
	require.NoError(t, err)
}

// checkGatewayConfigAnnotation verifies that the GatewayClassConfig was
// correctly serialized into the expected annotation on the Gateway.
func checkGatewayConfigAnnotation(t *testing.T, g *gateway.Gateway, gcc *apigwv1alpha1.GatewayClassConfig) {
	t.Helper()

	expectedCfg, err := json.Marshal(gcc.Spec)
	require.NoError(t, err)

	actualCfg, ok := g.Annotations[`api-gateway.consul.hashicorp.com/config`]
	assert.True(t, ok)
	assert.Equal(t, string(expectedCfg), actualCfg)
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
	}, checkTimeout, checkInterval, message)
}

func checkTCPRoute(t *testing.T, port int, expected string, message string) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: port,
		})
		if err != nil {
			return false
		}
		data, err := io.ReadAll(conn)
		if err != nil {
			return false
		}
		return strings.HasPrefix(string(data), expected)
	}, checkTimeout, checkInterval, message)
}

func checkTCPTLSRoute(t *testing.T, port int, config *tls.Config, expected string, message string) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: port,
		})
		if err != nil {
			return false
		}
		tlsConn := tls.Client(conn, config)
		data, err := io.ReadAll(tlsConn)

		if err != nil {
			t.Log(err)
			return strings.HasPrefix(err.Error(), expected)
		}

		return strings.HasPrefix(string(data), expected)
	}, checkTimeout, checkInterval, message)
}

func serviceType(v core.ServiceType) *core.ServiceType {
	return &v
}
