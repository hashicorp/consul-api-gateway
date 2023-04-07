// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build e2e

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
	"golang.org/x/exp/slices"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul/api"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/testing/e2e"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
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
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			// Create a GatewayClassConfig
			useHostPorts := false
			firstConfig, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts: &useHostPorts,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			oldUseHostPorts := firstConfig.Spec.UseHostPorts

			// Create an HTTPS Gateway Listener to pass when creating gateways
			httpsListener := createHTTPSListener(ctx, t, 443)

			// Create a Gateway and wait for it to be ready
			firstGatewayName := envconf.RandomName("gw", 16)
			firstGateway := createGateway(ctx, t, resources, firstGatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, firstGatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, firstGatewayName, namespace, firstConfig)

			// Modify GatewayClassConfig used for Gateway
			secondConfig := &apigwv1alpha1.GatewayClassConfig{}
			require.NoError(t, resources.Get(ctx, firstConfig.Name, namespace, secondConfig))

			newUseHostPorts := !oldUseHostPorts
			secondConfig.Spec.UseHostPorts = newUseHostPorts
			require.NoError(t, resources.Update(ctx, secondConfig))

			// Create a second Gateway and wait for it to be ready
			secondGatewayName := envconf.RandomName("gw", 16)
			secondGateway := createGateway(ctx, t, resources, secondGatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, secondGatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// Verify that 1st Gateway retains initial GatewayClassConfig and 2nd Gateway retains updated GatewayClassConfig
			checkGatewayConfigAnnotation(ctx, t, resources, firstGatewayName, namespace, firstConfig)
			checkGatewayConfigAnnotation(ctx, t, resources, secondGatewayName, namespace, secondConfig)

			assert.NoError(t, resources.Delete(ctx, firstGateway))
			assert.NoError(t, resources.Delete(ctx, secondGateway))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayWithoutNamespaceMirroring(t *testing.T) {
	feature := features.New("gateway admission").
		Assess("gateway sync without namespace mirroring", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			// Disable namespace mirroring
			ctx, err := e2e.SetNamespaceMirroring(false)(ctx, nil)
			require.NoError(t, err)

			useHostPorts := false
			gcc, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts: &useHostPorts,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create an HTTPS Gateway Listener to pass when creating gateways
			httpsListener := createHTTPSListener(ctx, t, 443)

			// Create a Gateway and wait for it to be ready
			// This will attempt to sync to a randomly generated Consul desintation namespace
			firstGatewayName := envconf.RandomName("gw", 16)
			firstGateway := createGateway(ctx, t, resources, firstGatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, firstGatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, firstGatewayName, namespace, gcc)

			// Set a different Consul destination namespace
			defaultNamespace := ""
			ctx, err = e2e.SetConsulNamespace(&defaultNamespace)(ctx, nil)
			require.NoError(t, err)

			// Create a second Gateway and wait for it to be ready
			secondGatewayName := envconf.RandomName("gw", 16)
			secondGateway := createGateway(ctx, t, resources, secondGatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, secondGatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, secondGatewayName, namespace, gcc)

			// Set a different Consul destination namespace
			defaultEnterpriseNamespace := "default"
			ctx, err = e2e.SetConsulNamespace(&defaultEnterpriseNamespace)(ctx, nil)
			require.NoError(t, err)

			// Create a third Gateway and wait for it to be ready
			thirdGatewayName := envconf.RandomName("gw", 16)
			thirdGateway := createGateway(ctx, t, resources, thirdGatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, thirdGatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, thirdGatewayName, namespace, gcc)

			// Cleanup
			assert.NoError(t, resources.Delete(ctx, firstGateway))
			assert.NoError(t, resources.Delete(ctx, secondGateway))
			assert.NoError(t, resources.Delete(ctx, thirdGateway))

			// Re-enable namespace mirroring
			ctx, err = e2e.SetNamespaceMirroring(true)(ctx, nil)
			require.NoError(t, err)

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayWithReplicas(t *testing.T) {
	feature := features.New("gateway class config configure instances").
		Assess("gateway is created with appropriate number of replicas set", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			var numberOfReplicas int32 = 3

			useHostPorts := false
			gcc, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts:     &useHostPorts,
				DefaultInstances: &numberOfReplicas,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create a Gateway and wait for it to be ready
			gatewayName := envconf.RandomName("gw", 16)
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{createHTTPSListener(ctx, t, 443)})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, gatewayName, namespace, gcc)

			// Fetch the deployment created by the gateway and check the number of replicas
			deployment := &apps.Deployment{}
			assert.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			assert.Equal(t, numberOfReplicas, *deployment.Spec.Replicas)

			// Cleanup
			assert.NoError(t, resources.Delete(ctx, gw))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayWithReplicasCanScale(t *testing.T) {
	feature := features.New("gateway class config doesn't override manual scaling").
		Assess("gateway deployment doesn't get overriden with kubectl scale operation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			var initialReplicas int32 = 3
			var finalReplicas int32 = 8

			useHostPorts := false
			gcc, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts:     &useHostPorts,
				DefaultInstances: &initialReplicas,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create a Gateway and wait for it to be ready
			gatewayName := envconf.RandomName("gw", 16)
			gateway := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{createHTTPSListener(ctx, t, 443)})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, gatewayName, namespace, gcc)

			// Fetch the deployment created by the gateway and check the number of replicas
			deployment := &apps.Deployment{}
			assert.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			assert.Equal(t, initialReplicas, *deployment.Spec.Replicas)

			// Manually set the number of replicas on the deployment
			deployment.Spec.Replicas = &finalReplicas
			assert.NoError(t, resources.Update(ctx, deployment))

			// Double check that the update wasn't overridden by the controller
			assert.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			assert.Equal(t, finalReplicas, *deployment.Spec.Replicas)

			assert.NoError(t, resources.Delete(ctx, gateway))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayWithReplicasRespectMinMax(t *testing.T) {
	t.Parallel()
	feature := features.New("gateway class config min max fields are respected").
		Assess("gateway deployment min maxes appropriately", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			var initialReplicas int32 = 3
			var minReplicas int32 = 2
			var maxReplicas int32 = 8
			exceedsMin := minReplicas - 1
			exceedsMax := maxReplicas + 1
			useHostPorts := false

			// Create a GatewayClassConfig
			gatewayClassConfig, gatewayClass := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts:     &useHostPorts,
				DefaultInstances: &initialReplicas,
				MaxInstances:     &maxReplicas,
				MinInstances:     &minReplicas,
			})

			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gatewayClass.Name, namespace, conditionAccepted), 30*time.Second, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create an HTTPS Gateway Listener to pass when creating gateways
			httpsListener := createHTTPSListener(ctx, t, 443)

			// Create a Gateway and wait for it to be ready
			gatewayName := envconf.RandomName("gw", 16)
			gateway := createGateway(ctx, t, resources, gatewayName, namespace, gatewayClass, []gwv1beta1.Listener{httpsListener})

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, gatewayName, namespace, gatewayClassConfig)

			// Fetch the deployment created by the gateway and check the number of replicas
			deployment := &apps.Deployment{}
			require.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			assert.Equal(t, initialReplicas, *deployment.Spec.Replicas)

			// Scale the deployment up
			deployment.Spec.Replicas = &exceedsMax
			assert.NoError(t, resources.Update(ctx, deployment))

			// Double check that replicas was set appropriately
			assert.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			assert.Eventually(t, deploymentReplicasSetAsExpected(ctx, resources, gatewayName, namespace, maxReplicas), 30*time.Second, checkInterval, "replicas not scaled down to max in the alloted time")

			// Scale the deployment down
			assert.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			deployment.Spec.Replicas = &exceedsMin
			assert.NoError(t, resources.Update(ctx, deployment))

			// Double check that replicas was set appropriately
			assert.NoError(t, resources.Get(ctx, gatewayName, namespace, deployment))
			assert.Eventually(t, deploymentReplicasSetAsExpected(ctx, resources, gatewayName, namespace, minReplicas), 30*time.Second, checkInterval, "replicas not scaled up to min in the alloted time")

			assert.NoError(t, resources.Delete(ctx, gateway))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayBasic(t *testing.T) {
	feature := features.New("gateway admission").
		Assess("basic admission and status updates", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)
			gatewayName := envconf.RandomName("gw", 16)

			useHostPorts := false
			gcc, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts: &useHostPorts,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			httpsListener := createHTTPSListener(ctx, t, 443)
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})

			require.Eventually(t, func() bool {
				err := resources.Get(ctx, gatewayName, namespace, &apps.Deployment{})
				return err == nil
			}, checkTimeout, checkInterval, "no deployment found in the allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			checkGatewayConfigAnnotation(ctx, t, resources, gatewayName, namespace, gcc)

			// check for the service being registered
			client := e2e.ConsulClient(ctx)
			t.Log("k8s namespace:", e2e.Namespace(ctx))
			t.Log("consul namespace:", e2e.ConsulNamespace(ctx))
			t.Log("mirroring:", e2e.NamespaceMirroring(ctx))

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
				return status == api.HealthPassing
			}, checkTimeout, checkInterval, "no healthy consul service found in the allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			err := resources.Delete(ctx, gw)
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", &api.QueryOptions{
					Namespace: e2e.ConsulNamespace(ctx),
				})
				if err != nil {
					fmt.Printf("ERROR: %#v", err)
					return false
				}
				fmt.Printf("SERVICES: %d\n", len(services))
				for _, service := range services {
					fmt.Printf("%#v\n", service)
				}
				return len(services) == 0
			}, checkTimeout, checkInterval, "consul service not deregistered in the allotted time")

			return ctx
		}).
		Assess("background sync into consul", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)
			gatewayName := envconf.RandomName("gw", 16)

			useHostPorts := false
			gcc, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts: &useHostPorts,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			httpsListener := createHTTPSListener(ctx, t, 443)
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})

			require.Eventually(t, func() bool {
				err := resources.Get(ctx, gatewayName, namespace, &apps.Deployment{})
				return err == nil
			}, checkTimeout, checkInterval, "no deployment found in the allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			checkGatewayConfigAnnotation(ctx, t, resources, gatewayName, namespace, gcc)

			client := e2e.ConsulClient(ctx)
			queryNamespace := &api.QueryOptions{Namespace: e2e.ConsulNamespace(ctx)}

			// Verify gateway is healthy
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// Check for service and config entries in Consul
			var service *api.CatalogService
			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", queryNamespace)
				if err == nil && len(services) == 1 && services[0].Checks.AggregatedStatus() == api.HealthPassing {
					service = services[0]
					return true
				}
				return false
			}, checkTimeout, checkInterval, "no healthy consul service found in the allotted time")
			require.Eventually(t, func() bool {
				entries, _, err := client.ConfigEntries().List(api.IngressGateway, queryNamespace)
				return err == nil && len(entries) == 1 && entries[0].GetName() == gatewayName
			}, 5*time.Minute, checkInterval, "ingress-gateway config-entry not created in allotted time")

			// De-register Consul service
			_, err := client.Catalog().Deregister(&api.CatalogDeregistration{
				Node:      service.Node,
				ServiceID: service.ServiceID,
				Namespace: service.Namespace,
			}, &api.WriteOptions{Namespace: service.Namespace})
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", queryNamespace)
				return err == nil && len(services) == 0
			}, 5*time.Minute, checkInterval, "service still returned after de-registering")

			// Delete ingress-gateway config-entry
			_, err = client.ConfigEntries().Delete(api.IngressGateway, gatewayName, &api.WriteOptions{Namespace: e2e.ConsulNamespace(ctx)})
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				entries, _, err := client.ConfigEntries().List(api.IngressGateway, queryNamespace)
				return err == nil && len(entries) == 0
			}, 5*time.Minute, checkInterval, "ingress-gateway config entry still returned after deleting")

			// Check to make sure the controller recreates the service and config-entry in the background.
			assert.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", queryNamespace)
				return err == nil && len(services) == 1
			}, 5*time.Minute, checkInterval, "service not recreated after delete in allotted time")
			assert.Eventually(t, func() bool {
				entry, _, err := client.ConfigEntries().Get(api.IngressGateway, gatewayName, queryNamespace)
				return err == nil && entry != nil
			}, 5*time.Minute, checkInterval, "ingress-gateway config-entry not recreated after delete in allotted time")

			// Clean up
			require.NoError(t, resources.Delete(ctx, gw))
			assert.Eventually(t, func() bool {
				services, _, err := client.Catalog().Service(gatewayName, "", queryNamespace)
				return err == nil && len(services) == 0
			}, checkTimeout, checkInterval, "gateway not deleted in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestServiceListeners(t *testing.T) {
	feature := features.New("service updates").
		Assess("port exposure for updated listeners", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			gatewayName := envconf.RandomName("gw", 16)

			useHostPorts := false
			_, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts: &useHostPorts,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			httpsListener := createHTTPSListener(ctx, t, 443)
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})

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

			namespace := e2e.Namespace(ctx)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)

			prefixMatch := gwv1alpha2.PathMatchPathPrefix
			headerMatch := gwv1alpha2.HeaderMatchExact

			resources := cfg.Client().Resources(namespace)

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			checkPort := e2e.HTTPFlattenedPort(ctx)
			httpsListener := createHTTPSListener(ctx, t, gwv1beta1.PortNumber(checkPort))
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			port := gwv1alpha2.PortNumber(serviceOne.Spec.Ports[0].Port)
			path := "/"
			route := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeOneName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.foo", "test.example"},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &prefixMatch,
								Value: &path,
							},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceOne.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			port = gwv1alpha2.PortNumber(serviceTwo.Spec.Ports[0].Port)
			path = "/v2"
			route = &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeTwoName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.foo"},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &prefixMatch,
								Value: &path,
							},
						}, {
							Headers: []gwv1alpha2.HTTPHeaderMatch{{
								Type:  &headerMatch,
								Name:  gwv1alpha2.HTTPHeaderName("x-v2"),
								Value: "v2",
							}},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceTwo.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			checkRoute(t, checkPort, "/v2/test", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceTwo.Name,
			}, map[string]string{
				"Host": "test.foo",
				"x-v2": "v2",
			}, "service two not routable in allotted time")
			checkRoute(t, checkPort, "/v2/test", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceTwo.Name,
			}, map[string]string{
				"Host": "test.foo",
			}, "service two not routable in allotted time")
			checkRoute(t, checkPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceTwo.Name,
			}, map[string]string{
				"Host": "test.foo",
				"x-v2": "v2",
			}, "service two with headers is not routable in allotted time")
			checkRoute(t, checkPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, map[string]string{
				"Host": "test.foo",
			}, "service one not routable in allotted time")
			checkRoute(t, checkPort, "/v2/test", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, map[string]string{
				"Host": "test.example",
			}, "service one not routable in allotted time")

			err = resources.Delete(ctx, gw)
			require.NoError(t, err)

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestHTTPRoutePathRewrite(t *testing.T) {
	feature := features.New("http url path rewrite").
		Assess("prefix rewrite", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			invalidService, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			validService, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespace(ctx)
			gatewayName := envconf.RandomName("gw", 16)
			invalidRouteName := envconf.RandomName("route", 16)
			validRouteName := envconf.RandomName("route", 16)

			prefixMatch := gwv1alpha2.PathMatchPathPrefix

			resources := cfg.Client().Resources(namespace)

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			checkPort := e2e.HTTPFlattenedPort(ctx)
			httpsListener := createHTTPSListener(ctx, t, gwv1beta1.PortNumber(checkPort))
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			port := gwv1alpha2.PortNumber(invalidService.Spec.Ports[0].Port)
			validPath := "/foo"
			invalidPath := "/bar"
			invalidPrefixMatch := "/v1/invalid"
			validPrefixMatch := "/v1/api"
			invalidRoute := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      invalidRouteName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.foo"},

					Rules: []gwv1alpha2.HTTPRouteRule{{
						Filters: []gwv1alpha2.HTTPRouteFilter{
							{
								Type: gwv1alpha2.HTTPRouteFilterURLRewrite,
								URLRewrite: &gwv1alpha2.HTTPURLRewriteFilter{
									Path: &gwv1alpha2.HTTPPathModifier{
										Type:               gwv1alpha2.PrefixMatchHTTPPathModifier,
										ReplacePrefixMatch: &invalidPrefixMatch,
									},
								},
							},
						},
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &prefixMatch,
								Value: &invalidPath,
							},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(invalidService.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, invalidRoute)
			require.NoError(t, err)

			port = gwv1alpha2.PortNumber(validService.Spec.Ports[0].Port)
			validRoute := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      validRouteName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.foo"},

					Rules: []gwv1alpha2.HTTPRouteRule{{
						Filters: []gwv1alpha2.HTTPRouteFilter{
							{
								Type: gwv1alpha2.HTTPRouteFilterURLRewrite,
								URLRewrite: &gwv1alpha2.HTTPURLRewriteFilter{
									Path: &gwv1alpha2.HTTPPathModifier{
										Type:               gwv1alpha2.PrefixMatchHTTPPathModifier,
										ReplacePrefixMatch: &validPrefixMatch,
									},
								},
							},
						},
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &prefixMatch,
								Value: &validPath,
							},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(validService.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, validRoute)
			require.NoError(t, err)

			checkRoute(t, checkPort, invalidPath, httpResponse{
				StatusCode: http.StatusOK,
				Body:       invalidService.Name,
			}, map[string]string{
				"Host": "test.foo",
			}, "invalid not routable in allotted time")
			checkRoute(t, checkPort, validPath, httpResponse{
				StatusCode: http.StatusOK,
				Body:       validService.Name,
			}, map[string]string{
				"Host": "test.foo",
			}, "valid service not routable in allotted time")

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

			namespace := e2e.Namespace(ctx)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route-1", 16)
			routeTwoName := envconf.RandomName("route-2", 16)
			routeThreeName := envconf.RandomName("route-3", 16)

			resources := cfg.Client().Resources(namespace)

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			httpsListener := createHTTPSListener(ctx, t, gwv1beta1.PortNumber(e2e.HTTPPort(ctx)))
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// Route 1 routes /v1 to service 1
			port := gwv1alpha2.PortNumber(serviceOne.Spec.Ports[0].Port)
			path := "/v1"
			pathMatch := gwv1alpha2.PathMatchExact
			routeOne := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeOneName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceOne.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			require.NoError(t, resources.Create(ctx, routeOne))

			// Route 2 routes /v2 to service 2 and / to services 4 and 5
			port = gwv1alpha2.PortNumber(serviceTwo.Spec.Ports[0].Port)
			portFour := gwv1alpha2.PortNumber(serviceFour.Spec.Ports[0].Port)
			portFive := gwv1alpha2.PortNumber(serviceFive.Spec.Ports[0].Port)
			path = "/v2"
			route := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeTwoName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceTwo.Name),
									Port: &port,
								},
							},
						}},
					}, {
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceFour.Name),
									Port: &portFour,
								},
							},
						}, {
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceFive.Name),
									Port: &portFive,
								},
							},
						}},
					}},
				},
			}
			require.NoError(t, resources.Create(ctx, route))

			// Route 3 routes /v3 to service 3
			port = gwv1alpha2.PortNumber(serviceThree.Spec.Ports[0].Port)
			path = "/v3"
			headerMatch := gwv1alpha2.HeaderMatchExact
			route = &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeThreeName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.host"},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
							Headers: []gwv1alpha2.HTTPHeaderMatch{{
								Type:  &headerMatch,
								Name:  gwv1alpha2.HTTPHeaderName("x-v3"),
								Value: "v3",
							}},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceThree.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			require.NoError(t, resources.Create(ctx, route))

			// Verify that routes are all working
			checkPort := e2e.HTTPPort(ctx)
			checkRoute(t, checkPort, "/v1", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, nil, "service one not routable in allotted time")
			checkRoute(t, checkPort, "/v2", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceTwo.Name,
			}, nil, "service two not routable in allotted time")
			checkRoute(t, checkPort, "/v3", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceThree.Name,
			}, map[string]string{
				"x-v3": "v3",
				"Host": "test.host",
			}, "service three not routable in allotted time")
			checkRoute(t, checkPort, "/v3", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceFour.Name,
			}, nil, "service four not routable in allotted time")
			checkRoute(t, checkPort, "/v3", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceFive.Name,
			}, nil, "service five not routable in allotted time")

			// Delete service 1 and verify that everything else continues working
			require.NoError(t, resources.Delete(ctx, serviceOne))
			checkRoute(t, checkPort, "/v1", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceFour.Name,
			}, nil, "after route deletion service four not routable in allotted time")
			checkRoute(t, checkPort, "/v1", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceFive.Name,
			}, nil, "after route deletion service five not routable in allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionInSync), checkTimeout, checkInterval, "gateway not synced in the allotted time")

			// Verify config entry for Gateway exists in Consul
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

			// Delete the gateway
			err = resources.Delete(ctx, gw)
			require.NoError(t, err)

			// Verify config entry for Gateway removed in Consul
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
		}).
		Assess("reconcile on service change", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			resources := cfg.Client().Resources(namespace)

			// Create GatewayClass
			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create Gateway
			gatewayName := envconf.RandomName("gateway", 16)
			httpsListener := createHTTPSListener(ctx, t, gwv1beta1.PortNumber(e2e.HTTPPort(ctx)))
			createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{httpsListener})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// Create Service
			service, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			// Create HTTPRoute
			routeName := envconf.RandomName("route", 16)
			port := gwv1alpha2.PortNumber(service.Spec.Ports[0].Port)
			path := "/"
			pathMatch := gwv1alpha2.PathMatchExact
			routeOne := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: routeName},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{Name: gwv1alpha2.ObjectName(gatewayName)}},
					},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						Matches: []gwv1alpha2.HTTPRouteMatch{{
							Path: &gwv1alpha2.HTTPPathMatch{
								Type:  &pathMatch,
								Value: &path,
							},
						}},
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(service.Name),
									Port: &port,
								},
							},
						}},
					}},
				},
			}
			require.NoError(t, resources.Create(ctx, routeOne))

			// Verify everything works
			checkRoute(t, e2e.HTTPPort(ctx), "/", httpResponse{StatusCode: http.StatusOK, Body: service.Name}, nil, "service not routable in allotted time")

			// Delete Service
			require.NoError(t, resources.Delete(ctx, service))

			// Verify HTTPRoute has updated its status
			check := createConditionsCheck([]meta.Condition{
				{
					Type: rstatus.RouteConditionResolvedRefs, Status: "False", Reason: rstatus.RouteConditionReasonBackendNotFound,
				},
			})
			require.Eventually(t, httpRouteStatusCheck(ctx, resources, gatewayName, routeName, namespace, check), checkTimeout, checkInterval, "route status not set in allotted time")

			// Re-create Service
			service.SetResourceVersion("")
			require.NoError(t, resources.Create(ctx, service))

			// Verify HTTPRoute has updated its status
			check = createConditionsCheck([]meta.Condition{
				{
					Type: rstatus.RouteConditionResolvedRefs, Status: "True", Reason: rstatus.RouteConditionReasonResolvedRefs,
				},
			})
			require.Eventually(t, httpRouteStatusCheck(ctx, resources, gatewayName, routeName, namespace, check), checkTimeout, checkInterval, "route status not set in allotted time")

			// Verify the HTTPRoute can find the Service
			checkRoute(t, e2e.HTTPPort(ctx), "/", httpResponse{StatusCode: http.StatusOK, Body: service.Name}, nil, "service not routable in allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestGatewayWithConsulNamespaceDoesntExist(t *testing.T) {
	feature := features.New("gateway class in k8s namespace where consul namespace not created yet, consul enterprise").
		Assess("gateway is created with appropriate number of replicas set", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			if !e2e.IsEnterprise() {
				t.Skip("Consul Namespaces only exist in consul enterprise")
			}

			namespace := e2e.Namespace(ctx)
			client := e2e.ConsulClient(ctx)

			// delete from consul is enterprise

			// delete namespace from consul
			fmt.Println("deleting from consul")
			_, err := client.Namespaces().Delete(namespace, nil)
			assert.NoError(t, err)

			resources := cfg.Client().Resources(namespace)

			useHostPorts := false
			gcc, gc := createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{
				UseHostPorts: &useHostPorts,
			})
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create a Gateway and wait for it to be ready
			gatewayName := envconf.RandomName("gw", 16)
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{createHTTPListener(ctx, t, 80)})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")
			checkGatewayConfigAnnotation(ctx, t, resources, gatewayName, namespace, gcc)
			checkCrossNamespacePolicyAppliedToNewNamespace(ctx, t, client, namespace)

			// Cleanup
			assert.NoError(t, resources.Delete(ctx, gw))

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

			namespace := e2e.Namespace(ctx)
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

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			gw := &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gwv1beta1.GatewaySpec{
					GatewayClassName: gwv1beta1.ObjectName(gc.Name),
					Listeners: []gwv1beta1.Listener{{
						Name:     "tcp",
						Port:     gwv1beta1.PortNumber(e2e.TCPPort(ctx)),
						Protocol: gwv1beta1.TCPProtocolType,
					}},
				},
			}
			err = resources.Create(ctx, gw)
			require.NoError(t, err)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// route 1
			portOne := gwv1alpha2.PortNumber(serviceOne.Spec.Ports[0].Port)
			portTwo := gwv1alpha2.PortNumber(serviceTwo.Spec.Ports[0].Port)
			portThree := gwv1alpha2.PortNumber(serviceThree.Spec.Ports[0].Port)
			routeOne := &gwv1alpha2.TCPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeOneName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.TCPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Rules: []gwv1alpha2.TCPRouteRule{{
						BackendRefs: []gwv1alpha2.BackendRef{{
							BackendObjectReference: gwv1alpha2.BackendObjectReference{
								Name: gwv1alpha2.ObjectName(serviceOne.Name),
								Port: &portOne,
							},
						}, {
							BackendObjectReference: gwv1alpha2.BackendObjectReference{
								Name: gwv1alpha2.ObjectName(serviceTwo.Name),
								Port: &portTwo,
							},
						}},
					}, {
						BackendRefs: []gwv1alpha2.BackendRef{{
							BackendObjectReference: gwv1alpha2.BackendObjectReference{
								Name: gwv1alpha2.ObjectName(serviceThree.Name),
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
				createConditionsCheck([]meta.Condition{
					{Type: "ResolvedRefs", Status: "False", Reason: "Errors"},
				}),
			), checkTimeout, checkInterval, "route status not set in allotted time")

			// route 2
			meshServiceGroup := gwv1alpha2.Group(apigwv1alpha1.Group)
			meshServiceKind := gwv1alpha2.Kind(apigwv1alpha1.MeshServiceKind)
			// this routes to service four
			route := &gwv1alpha2.TCPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      routeTwoName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.TCPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(gatewayName),
						}},
					},
					Rules: []gwv1alpha2.TCPRouteRule{{
						BackendRefs: []gwv1alpha2.BackendRef{{
							BackendObjectReference: gwv1alpha2.BackendObjectReference{
								Group: &meshServiceGroup,
								Kind:  &meshServiceKind,
								Name:  gwv1alpha2.ObjectName(meshServiceName),
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, route)
			require.NoError(t, err)

			checkPort := e2e.TCPPort(ctx)

			// only service 4 should be routable as we don't support routes with multiple rules or backend refs for TCP
			checkTCPRoute(t, checkPort, serviceFour.Name, false, "service four not routable in allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionInSync), checkTimeout, checkInterval, "gateway not synced in the allotted time")
			return ctx
		}).
		Assess("tls routing", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespace(ctx)
			gatewayName := envconf.RandomName("gw", 16)
			routeOneName := envconf.RandomName("route", 16)
			routeTwoName := envconf.RandomName("route", 16)
			listenerOneName := "tcp"
			listenerTwoName := "insecure"
			listenerOnePort := e2e.TCPTLSPort(ctx)
			listenerTwoPort := e2e.ExtraTCPTLSPort(ctx)

			gatewayNamespace := gwv1beta1.Namespace(namespace)
			resources := cfg.Client().Resources(namespace)

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			gw := &gwv1beta1.Gateway{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayName,
					Namespace: namespace,
				},
				Spec: gwv1beta1.GatewaySpec{
					GatewayClassName: gwv1beta1.ObjectName(gc.Name),
					Listeners: []gwv1beta1.Listener{
						{
							Name:     gwv1beta1.SectionName(listenerOneName),
							Port:     gwv1beta1.PortNumber(listenerOnePort),
							Protocol: gwv1beta1.TCPProtocolType,
							TLS: &gwv1beta1.GatewayTLSConfig{
								CertificateRefs: []gwv1beta1.SecretObjectReference{{
									Name:      "consul-server-cert",
									Namespace: &gatewayNamespace,
								}},
							},
						},
						{
							Name:     gwv1beta1.SectionName(listenerTwoName),
							Port:     gwv1beta1.PortNumber(listenerTwoPort),
							Protocol: gwv1beta1.TCPProtocolType,
							TLS: &gwv1beta1.GatewayTLSConfig{
								CertificateRefs: []gwv1beta1.SecretObjectReference{{
									Name:      "consul-server-cert",
									Namespace: &gatewayNamespace,
								}},
								Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
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

			createTCPRoute(ctx, t, resources, namespace, gatewayName, gwv1alpha2.SectionName(listenerOneName), routeOneName, serviceOne.Name, gwv1alpha2.PortNumber(serviceOne.Spec.Ports[0].Port))
			createTCPRoute(ctx, t, resources, namespace, gatewayName, gwv1alpha2.SectionName(listenerTwoName), routeTwoName, serviceTwo.Name, gwv1alpha2.PortNumber(serviceTwo.Spec.Ports[0].Port))

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
				MinVersion:         tls.VersionTLS10,
				MaxVersion:         tls.VersionTLS11,
			}, "remote error: tls: protocol version not supported", "connection not rejected with expected error in allotted time")

			// Service two listener overrides default config
			checkTCPTLSRoute(t, listenerTwoPort, &tls.Config{
				InsecureSkipVerify: true,
				CipherSuites:       []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA},
				MinVersion:         tls.VersionTLS10,
				MaxVersion:         tls.VersionTLS11,
			}, serviceTwo.Name, "service not routable in allotted time")

			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionInSync), checkTimeout, checkInterval, "gateway not synced in the allotted time")

			require.Eventually(t, listenerStatusCheck(ctx, resources, gatewayName, namespace, createListenerStatusConditionsFnCheck(conditionReady)), checkTimeout, checkInterval, "listeners not ready in the allotted time")

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestReferenceGrantLifecycle(t *testing.T) {
	feature := features.New("reference grant").
		Assess("route controllers watch reference grant changes", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)
			serviceTwo, err := e2e.DeployTCPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespace(ctx)
			gatewayName := envconf.RandomName("gw", 16)
			httpRouteName := envconf.RandomName("httproute", 16)
			httpRouteNamespace := envconf.RandomName("ns", 16)
			httpRouteRefGrantName := envconf.RandomName("refgrant", 16)
			tcpRouteName := envconf.RandomName("tcproute", 16)
			tcpRouteNamespace := envconf.RandomName("ns", 16)
			tcpRouteRefGrantName := envconf.RandomName("refgrant", 16)

			resources := cfg.Client().Resources(namespace)

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			httpCheckPort := e2e.HTTPReferenceGrantPort(ctx)
			tcpCheckPort := e2e.TCPReferenceGrantPort(ctx)

			// Allow routes to bind from a different namespace for testing
			// cross-namespace ReferenceGrant enforcement
			fromSelector := gwv1beta1.NamespacesFromSelector

			gwNamespace := gwv1beta1.Namespace(namespace)
			routeGatewayNamespace := gwv1alpha2.Namespace(namespace)
			gw := createGateway(ctx, t, resources, gatewayName, namespace, gc, []gwv1beta1.Listener{
				{
					Name:     "https",
					Port:     gwv1beta1.PortNumber(httpCheckPort),
					Protocol: gwv1beta1.HTTPSProtocolType,
					TLS: &gwv1beta1.GatewayTLSConfig{
						CertificateRefs: []gwv1beta1.SecretObjectReference{{
							Name:      "consul-server-cert",
							Namespace: &gwNamespace,
						}},
					},
					AllowedRoutes: &gwv1beta1.AllowedRoutes{
						Namespaces: &gwv1beta1.RouteNamespaces{
							From: &fromSelector,
							Selector: &meta.LabelSelector{
								MatchExpressions: []meta.LabelSelectorRequirement{{
									Key:      "kubernetes.io/metadata.name",
									Operator: "In",
									Values:   []string{httpRouteNamespace},
								}},
							},
						},
					},
				},
				{
					Name:     "tcp",
					Port:     gwv1beta1.PortNumber(tcpCheckPort),
					Protocol: gwv1beta1.TCPProtocolType,
					AllowedRoutes: &gwv1beta1.AllowedRoutes{
						Namespaces: &gwv1beta1.RouteNamespaces{
							From: &fromSelector,
							Selector: &meta.LabelSelector{
								MatchExpressions: []meta.LabelSelectorRequirement{{
									Key:      "kubernetes.io/metadata.name",
									Operator: "In",
									Values:   []string{tcpRouteNamespace},
								}},
							},
						},
					},
				},
			})
			require.Eventually(t, gatewayStatusCheck(ctx, resources, gatewayName, namespace, conditionReady), checkTimeout, checkInterval, "no gateway found in the allotted time")

			// Create a different namespace for the HTTPRoute
			httpNs := &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name: httpRouteNamespace,
				},
			}
			err = resources.Create(ctx, httpNs)
			require.NoError(t, err)

			// Create a different namespace for the TCPRoute
			tcpNs := &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name: tcpRouteNamespace,
				},
			}
			err = resources.Create(ctx, tcpNs)
			require.NoError(t, err)

			httpPort := gwv1alpha2.PortNumber(serviceOne.Spec.Ports[0].Port)
			httpRoute := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      httpRouteName,
					Namespace: httpRouteNamespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name:      gwv1alpha2.ObjectName(gatewayName),
							Namespace: &routeGatewayNamespace,
						}},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.foo"},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name:      gwv1alpha2.ObjectName(serviceOne.Name),
									Namespace: &routeGatewayNamespace,
									Port:      &httpPort,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, httpRoute)
			require.NoError(t, err)

			// Expect that HTTPRoute sets
			// ResolvedRefs{ status: False, reason: RefNotPermitted }
			// due to missing ReferenceGrant for BackendRef in other namespace
			httpRouteStatusCheckRefNotPermitted := httpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				httpRouteName,
				httpRouteNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "False"},
					{Type: "ResolvedRefs", Status: "False", Reason: "RefNotPermitted"},
				}),
			)
			require.Eventually(t, httpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")

			tcpNamespace := gwv1alpha2.Namespace(namespace)
			tcpPort := gwv1alpha2.PortNumber(serviceTwo.Spec.Ports[0].Port)
			tcpRoute := &gwv1alpha2.TCPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      tcpRouteName,
					Namespace: tcpRouteNamespace,
				},
				Spec: gwv1alpha2.TCPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name:      gwv1alpha2.ObjectName(gatewayName),
							Namespace: &tcpNamespace,
						}},
					},
					Rules: []gwv1alpha2.TCPRouteRule{{
						BackendRefs: []gwv1alpha2.BackendRef{{
							BackendObjectReference: gwv1alpha2.BackendObjectReference{
								Name:      gwv1alpha2.ObjectName(serviceTwo.Name),
								Namespace: &tcpNamespace,
								Port:      &tcpPort,
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, tcpRoute)
			require.NoError(t, err)

			serviceOneObjectName := gwv1alpha2.ObjectName(serviceOne.Name)

			tcpRouteStatusCheckRefNotPermitted := tcpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				tcpRouteName,
				tcpRouteNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "False"},
					{Type: "ResolvedRefs", Status: "False", Reason: "RefNotPermitted"},
				}),
			)

			// create ReferencePolicy allowing HTTPRoute BackendRef
			httpRouteReferencePolicy := &gwv1alpha2.ReferencePolicy{
				ObjectMeta: meta.ObjectMeta{
					Name:      httpRouteRefGrantName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "HTTPRoute",
						Namespace: gwv1alpha2.Namespace(httpRouteNamespace),
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Group: "",
						Kind:  "Service",
						Name:  &serviceOneObjectName,
					}},
				},
			}
			err = resources.Create(ctx, httpRouteReferencePolicy)
			require.NoError(t, err)

			// Expect that HTTPRoute sets
			// ResolvedRefs{ status: True, reason: ResolvedRefs }
			// now that ReferencePolicy allows BackendRef in other namespace
			require.Eventually(t, httpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				httpRouteName,
				httpRouteNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "True"},
					{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"},
				}),
			), checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")

			// Check that HTTPRoute is successfully resolved and routing traffic
			checkRoute(t, httpCheckPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, map[string]string{
				"Host": "test.foo",
			}, "service one not routable in allotted time")

			// Expect that TCPRoute still sets
			// ResolvedRefs{ status: False, reason: RefNotPermitted }
			// due to missing ReferenceGrant for BackendRef in other namespace
			require.Eventually(t, tcpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "TCPRoute status not set in allotted time")

			// Delete HTTPRoute ReferencePolicy, check for RefNotPermitted again
			// Check that Gateway has cleaned up stale route and is no longer routing traffic
			err = resources.Delete(ctx, httpRouteReferencePolicy)
			require.NoError(t, err)
			require.Eventually(t, httpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")
			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				gatewayName,
				namespace,
				listenerAttachedRoutes(0, "https"),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")
			// TODO: when implementation is updated, this should be refactored to check for a 404 status code
			// instead of a connection error
			checkRouteError(t, httpCheckPort, "/", map[string]string{
				"Host": "test.foo",
			}, "service one still routable in allotted time")

			// create ReferenceGrant allowing HTTPRoute BackendRef
			httpRouteReferenceGrant := &gwv1alpha2.ReferenceGrant{
				ObjectMeta: meta.ObjectMeta{
					Name:      httpRouteRefGrantName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "HTTPRoute",
						Namespace: gwv1alpha2.Namespace(httpRouteNamespace),
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Group: "",
						Kind:  "Service",
						Name:  &serviceOneObjectName,
					}},
				},
			}
			err = resources.Create(ctx, httpRouteReferenceGrant)
			require.NoError(t, err)

			// Expect that HTTPRoute sets
			// ResolvedRefs{ status: True, reason: ResolvedRefs }
			// now that ReferenceGrant allows BackendRef in other namespace
			require.Eventually(t, httpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				httpRouteName,
				httpRouteNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "True"},
					{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"},
				}),
			), checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")

			// Check that HTTPRoute is successfully resolved and routing traffic
			checkRoute(t, httpCheckPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, map[string]string{
				"Host": "test.foo",
			}, "service one not routable in allotted time")

			// Expect that TCPRoute still sets
			// ResolvedRefs{ status: False, reason: RefNotPermitted }
			// due to missing ReferenceGrant for BackendRef in other namespace
			require.Eventually(t, tcpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "TCPRoute status not set in allotted time")

			serviceTwoObjectName := gwv1alpha2.ObjectName(serviceTwo.Name)

			// create ReferencePolicy allowing TCPRoute BackendRef
			tcpRouteReferencePolicy := &gwv1alpha2.ReferencePolicy{
				ObjectMeta: meta.ObjectMeta{
					Name:      tcpRouteRefGrantName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "TCPRoute",
						Namespace: gwv1alpha2.Namespace(tcpRouteNamespace),
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Group: "",
						Kind:  "Service",
						Name:  &serviceTwoObjectName,
					}},
				},
			}
			err = resources.Create(ctx, tcpRouteReferencePolicy)
			require.NoError(t, err)

			// Expect that TCPRoute sets
			// ResolvedRefs{ status: True, reason: ResolvedRefs }
			// now that ReferencePolicy allows BackendRef in other namespace
			require.Eventually(t, tcpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				tcpRouteName,
				tcpRouteNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "True"},
					{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"},
				}),
			), checkTimeout, checkInterval, "TCPRoute status not set in allotted time")

			// Check that TCPRoute is successfully resolved and routing traffic
			checkTCPRoute(t, tcpCheckPort, serviceTwo.Name, false, "service two not routable in allotted time")

			// Delete TCPRoute ReferencePolicy, check for RefNotPermitted again
			// Check that Gateway has cleaned up stale route and is no longer routing traffic
			err = resources.Delete(ctx, tcpRouteReferencePolicy)
			require.NoError(t, err)
			require.Eventually(t, tcpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "TCPRoute status not set in allotted time")
			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				gatewayName,
				namespace,
				listenerAttachedRoutes(0, "tcp"),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")
			// The following error is logged but doesn't seem to get propagated up to be able to check it properly
			// [WARN]  [core]grpc: Server.Serve failed to complete security handshake: remote error: tls: unknown certificate authority
			checkTCPRoute(t, tcpCheckPort, "", true, "service two still routable in allotted time")

			// create ReferenceGrant allowing TCPRoute BackendRef
			tcpRouteReferenceGrant := &gwv1alpha2.ReferenceGrant{
				ObjectMeta: meta.ObjectMeta{
					Name:      tcpRouteRefGrantName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "TCPRoute",
						Namespace: gwv1alpha2.Namespace(tcpRouteNamespace),
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Group: "",
						Kind:  "Service",
						Name:  &serviceTwoObjectName,
					}},
				},
			}
			err = resources.Create(ctx, tcpRouteReferenceGrant)
			require.NoError(t, err)

			// Expect that TCPRoute sets
			// ResolvedRefs{ status: True, reason: ResolvedRefs }
			// now that ReferenceGrant allows BackendRef in other namespace
			require.Eventually(t, tcpRouteStatusCheck(
				ctx,
				resources,
				gatewayName,
				tcpRouteName,
				tcpRouteNamespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "True"},
					{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"},
				}),
			), checkTimeout, checkInterval, "TCPRoute status not set in allotted time")

			// Check that TCPRoute is successfully resolved and routing traffic
			checkTCPRoute(t, tcpCheckPort, serviceTwo.Name, false, "service two not routable in allotted time")

			// Delete TCPRoute ReferenceGrant, check for RefNotPermitted again
			// Check that Gateway has cleaned up stale route and is no longer routing traffic
			err = resources.Delete(ctx, tcpRouteReferenceGrant)
			require.NoError(t, err)
			require.Eventually(t, tcpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "TCPRoute status not set in allotted time")
			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				gatewayName,
				namespace,
				listenerAttachedRoutes(0, "tcp"),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")
			// The following error is logged but doesn't seem to get propagated up to be able to check it properly
			// [WARN]  [core]grpc: Server.Serve failed to complete security handshake: remote error: tls: unknown certificate authority
			checkTCPRoute(t, tcpCheckPort, "", true, "service two still routable in allotted time")

			// Delete HTTPRoute ReferenceGrant, check for RefNotPermitted again
			// Check that Gateway has cleaned up stale route and is no longer routing traffic
			err = resources.Delete(ctx, httpRouteReferenceGrant)
			require.NoError(t, err)
			require.Eventually(t, httpRouteStatusCheckRefNotPermitted, checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")
			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				gatewayName,
				namespace,
				listenerAttachedRoutes(0, "https"),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")
			// TODO: when implementation is updated, this should be refactored to check for a 404 status code
			// instead of a connection error
			checkRouteError(t, httpCheckPort, "/", map[string]string{
				"Host": "test.foo",
			}, "service one still routable in allotted time")

			err = resources.Delete(ctx, gw)
			require.NoError(t, err)

			return ctx
		}).
		Assess("gateway controller watches reference grant changes", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := e2e.Namespace(ctx)
			gatewayNamespace := namespace
			gatewayName := envconf.RandomName("gw", 16)
			certNamespace := envconf.RandomName("ns", 16)
			certName := "consul-server-cert"
			gatewayRefGrantName := envconf.RandomName("refgrant", 16)

			resources := cfg.Client().Resources(namespace)

			// Make a copy of the certificate Secret in a different namespace for the Gateway to reference.
			// This is easier than creating the Gateway in a different namespace due to pre-installed ServiceAccount dependency.
			certCopy := &core.Secret{}
			require.NoError(t, resources.Get(ctx, certName, namespace, certCopy))
			certCopy.SetNamespace(certNamespace)
			certCopy.SetResourceVersion("")
			require.NoError(t, resources.Create(ctx, &core.Namespace{ObjectMeta: meta.ObjectMeta{Name: certNamespace}}))
			require.NoError(t, resources.Create(ctx, certCopy))

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), checkTimeout, checkInterval, "gatewayclass not accepted in the allotted time")

			fromSelector := gwv1beta1.NamespacesFromAll

			// Create a Gateway with a listener that has a CertificateRef to a different namespace
			certNamespaceTyped := gwv1beta1.Namespace(certNamespace)
			gw := createGateway(ctx, t, resources, gatewayName, gatewayNamespace, gc, []gwv1beta1.Listener{
				{
					Name:     "https",
					Port:     gwv1beta1.PortNumber(e2e.HTTPReferenceGrantPort(ctx)),
					Protocol: gwv1beta1.HTTPSProtocolType,
					TLS: &gwv1beta1.GatewayTLSConfig{
						CertificateRefs: []gwv1beta1.SecretObjectReference{{
							Name:      gwv1beta1.ObjectName(certName),
							Namespace: &certNamespaceTyped,
						}},
					},
					AllowedRoutes: &gwv1beta1.AllowedRoutes{
						Namespaces: &gwv1beta1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				},
			})

			// Expect that Gateway has expected error condition
			// due to missing ReferenceGrant for CertificateRef in other namespace
			gatewayConditionCheck := createConditionsCheck([]meta.Condition{{Type: "Ready", Status: "False", Reason: "ListenersNotValid"}})
			gatewayCheck := gatewayStatusCheck(ctx, resources, gatewayName, gatewayNamespace, gatewayConditionCheck)
			require.Eventually(t, gatewayCheck, checkTimeout, checkInterval, "Gateway status not set in allotted time")

			// Expect that Gateway listener has expected error condition
			// due to missing ReferenceGrant for CertificateRef in other namespace
			listenerConditionCheck := createListenerStatusConditionsCheck([]meta.Condition{{Type: "ResolvedRefs", Status: "False", Reason: "RefNotPermitted"}})
			listenerCheck := listenerStatusCheck(ctx, resources, gatewayName, gatewayNamespace, listenerConditionCheck)
			require.Eventually(t, listenerCheck, checkTimeout, checkInterval, "Gateway listener status not set in allotted time")

			// Create ReferencePolicy allowing Gateway CertificateRef
			certReferencePolicy := &gwv1alpha2.ReferencePolicy{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayRefGrantName,
					Namespace: string(certNamespace),
				},
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Namespace: gwv1alpha2.Namespace(gatewayNamespace),
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Group: "",
						Kind:  "Secret",
						Name:  nil,
					}},
				},
			}
			require.NoError(t, resources.Create(ctx, certReferencePolicy))

			// Expect that Gateway has expected success condition
			gatewayCheck = gatewayStatusCheck(ctx, resources, gatewayName, gatewayNamespace, conditionReady)
			require.Eventually(t, gatewayCheck, checkTimeout, checkInterval, "Gateway status not set in allotted time")

			// Expect that Gateway listener has expected success condition
			listenerConditionCheck = createListenerStatusConditionsCheck([]meta.Condition{{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"}})
			listenerCheck = listenerStatusCheck(ctx, resources, gatewayName, gatewayNamespace, listenerConditionCheck)
			require.Eventually(t, listenerCheck, checkTimeout, checkInterval, "Gateway listener status not set in allotted time")

			// Delete Gateway ReferencePolicy
			require.NoError(t, resources.Delete(ctx, certReferencePolicy))

			// Check for error status conditions again
			gatewayConditionCheck = createConditionsCheck([]meta.Condition{{Type: "Ready", Status: "False", Reason: "ListenersNotValid"}})
			gatewayCheck = gatewayStatusCheck(ctx, resources, gatewayName, gatewayNamespace, gatewayConditionCheck)
			require.Eventually(t, gatewayCheck, checkTimeout, checkInterval, "Gateway status not set in allotted time")

			listenerConditionCheck = createListenerStatusConditionsCheck([]meta.Condition{{Type: "ResolvedRefs", Status: "False", Reason: "RefNotPermitted"}})
			listenerCheck = listenerStatusCheck(ctx, resources, gatewayName, gatewayNamespace, listenerConditionCheck)
			require.Eventually(t, listenerCheck, checkTimeout, checkInterval, "Gateway listener status not set in allotted time")

			// Create ReferenceGrant allowing Gateway CertificateRef
			certReferenceGrant := &gwv1alpha2.ReferenceGrant{
				ObjectMeta: meta.ObjectMeta{
					Name:      gatewayRefGrantName,
					Namespace: string(certNamespace),
				},
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{{
						Group:     "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Namespace: gwv1alpha2.Namespace(gatewayNamespace),
					}},
					To: []gwv1alpha2.ReferenceGrantTo{{
						Group: "",
						Kind:  "Secret",
						Name:  nil,
					}},
				},
			}
			require.NoError(t, resources.Create(ctx, certReferenceGrant))

			// Expect that Gateway has expected success condition
			gatewayCheck = gatewayStatusCheck(ctx, resources, gatewayName, gatewayNamespace, conditionReady)
			require.Eventually(t, gatewayCheck, checkTimeout, checkInterval, "Gateway status not set in allotted time")

			// Expect that Gateway listener has expected success condition
			listenerConditionCheck = createListenerStatusConditionsCheck([]meta.Condition{{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"}})
			listenerCheck = listenerStatusCheck(ctx, resources, gatewayName, gatewayNamespace, listenerConditionCheck)
			require.Eventually(t, listenerCheck, checkTimeout, checkInterval, "Gateway listener status not set in allotted time")

			// Delete Gateway ReferenceGrant
			require.NoError(t, resources.Delete(ctx, certReferenceGrant))

			// Check for error status conditions again
			gatewayConditionCheck = createConditionsCheck([]meta.Condition{{Type: "Ready", Status: "False", Reason: "ListenersNotValid"}})
			gatewayCheck = gatewayStatusCheck(ctx, resources, gatewayName, gatewayNamespace, gatewayConditionCheck)
			require.Eventually(t, gatewayCheck, checkTimeout, checkInterval, "Gateway status not set in allotted time")

			listenerConditionCheck = createListenerStatusConditionsCheck([]meta.Condition{{Type: "ResolvedRefs", Status: "False", Reason: "RefNotPermitted"}})
			listenerCheck = listenerStatusCheck(ctx, resources, gatewayName, gatewayNamespace, listenerConditionCheck)
			require.Eventually(t, listenerCheck, checkTimeout, checkInterval, "Gateway listener status not set in allotted time")

			// Clean up
			require.NoError(t, resources.Delete(ctx, gw))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func TestRouteParentRefChange(t *testing.T) {
	feature := features.New("route parentref change").
		Assess("gateway behavior on route parentref change", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			serviceOne, err := e2e.DeployHTTPMeshService(ctx, cfg)
			require.NoError(t, err)

			namespace := e2e.Namespace(ctx)
			routeGatewayNamespace := gwv1alpha2.Namespace(namespace)
			resources := cfg.Client().Resources(namespace)

			_, gc := createGatewayClass(ctx, t, resources)
			require.Eventually(t, gatewayClassStatusCheck(ctx, resources, gc.Name, namespace, conditionAccepted), 30*time.Second, checkInterval, "gatewayclass not accepted in the allotted time")

			// Create a Gateway and wait for it to be ready
			firstGatewayName := envconf.RandomName("gw", 16)
			firstGatewayCheckPort := e2e.ParentRefChangeFirstGatewayPort(ctx)
			firstGateway := createGateway(
				ctx,
				t,
				resources,
				firstGatewayName,
				namespace,
				gc,
				[]gwv1beta1.Listener{createHTTPSListener(ctx, t, gwv1beta1.PortNumber(firstGatewayCheckPort))},
			)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, firstGatewayName, namespace, conditionReady), 30*time.Second, checkInterval, "no gateway found in the allotted time")

			// Create route with ParentRef targeting first gateway
			httpRouteName := envconf.RandomName("httproute", 16)
			httpPort := gwv1alpha2.PortNumber(serviceOne.Spec.Ports[0].Port)
			httpRoute := &gwv1alpha2.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name:      httpRouteName,
					Namespace: namespace,
				},
				Spec: gwv1alpha2.HTTPRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{
							Name: gwv1alpha2.ObjectName(firstGatewayName),
						}},
					},
					Rules: []gwv1alpha2.HTTPRouteRule{{
						BackendRefs: []gwv1alpha2.HTTPBackendRef{{
							BackendRef: gwv1alpha2.BackendRef{
								BackendObjectReference: gwv1alpha2.BackendObjectReference{
									Name: gwv1alpha2.ObjectName(serviceOne.Name),
									Port: &httpPort,
								},
							},
						}},
					}},
				},
			}
			err = resources.Create(ctx, httpRoute)
			require.NoError(t, err)

			// Check that route binds to listener successfully
			require.Eventually(t, httpRouteStatusCheck(
				ctx,
				resources,
				firstGatewayName,
				httpRouteName,
				namespace,
				createConditionsCheck([]meta.Condition{
					{Type: "Accepted", Status: "True"},
					{Type: "ResolvedRefs", Status: "True", Reason: "ResolvedRefs"},
				}),
			), checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")
			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				firstGatewayName,
				namespace,
				listenerAttachedRoutes(1),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")

			// Check that HTTPRoute is successfully resolved and routing traffic
			checkRoute(t, firstGatewayCheckPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, nil, "service one not routable in allotted time")

			// Create a second Gateway and wait for it to be ready
			secondGatewayName := envconf.RandomName("gw", 16)
			secondGatewayCheckPort := e2e.ParentRefChangeSecondGatewayPort(ctx)
			secondGateway := createGateway(
				ctx,
				t,
				resources,
				secondGatewayName,
				namespace,
				gc,
				[]gwv1beta1.Listener{createHTTPSListener(ctx, t, gwv1beta1.PortNumber(secondGatewayCheckPort))},
			)
			require.Eventually(t, gatewayStatusCheck(ctx, resources, secondGatewayName, namespace, conditionReady), 30*time.Second, checkInterval, "no gateway found in the allotted time")

			// Update httpRoute from remote, then add second gateway ParentRef
			require.NoError(t, resources.Get(ctx, httpRouteName, namespace, httpRoute))
			httpRoute.Spec.CommonRouteSpec.ParentRefs = []gwv1alpha2.ParentReference{
				{
					Name:      gwv1alpha2.ObjectName(firstGatewayName),
					Namespace: &routeGatewayNamespace,
				},
				{
					Name:      gwv1alpha2.ObjectName(secondGatewayName),
					Namespace: &routeGatewayNamespace,
				},
			}
			require.NoError(t, resources.Update(ctx, httpRoute))

			// Check that route binds to second gateway listener successfully
			require.Eventually(t, httpRouteStatusCheck(
				ctx,
				resources,
				secondGatewayName,
				httpRouteName,
				namespace,
				conditionAccepted,
			), checkTimeout, checkInterval, "HTTPRoute status not set in allotted time")
			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				secondGatewayName,
				namespace,
				listenerAttachedRoutes(1),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")

			// Check that HTTPRoute is still routing traffic from first gateway
			checkRoute(t, firstGatewayCheckPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, nil, "service one not routable from first gateway in allotted time")

			checkRoute(t, secondGatewayCheckPort, "/", httpResponse{
				StatusCode: http.StatusOK,
				Body:       serviceOne.Name,
			}, nil, "service one not returning expected error from second gateway in allotted time")

			// Update httpRoute from remote, then remove first gateway ParentRef
			require.NoError(t, resources.Get(ctx, httpRouteName, namespace, httpRoute))
			httpRoute.Spec.CommonRouteSpec.ParentRefs = []gwv1alpha2.ParentReference{{
				Name:      gwv1alpha2.ObjectName(secondGatewayName),
				Namespace: &routeGatewayNamespace,
			}}
			require.NoError(t, resources.Update(ctx, httpRoute))

			// Check that route unbinds from first gateway listener successfully
			require.Eventually(t, func() bool {
				updated := &gwv1alpha2.HTTPRoute{}
				if err := resources.Get(ctx, httpRouteName, namespace, updated); err != nil {
					return false
				}
				for _, status := range updated.Status.Parents {
					if string(status.ParentRef.Name) == firstGatewayName {
						return false
					}
				}
				return true
			}, checkTimeout, checkInterval, "HTTPRoute status not unset in allotted time")

			require.Eventually(t, listenerStatusCheck(
				ctx,
				resources,
				firstGatewayName,
				namespace,
				listenerAttachedRoutes(0),
			), checkTimeout, checkInterval, "listeners not ready in the allotted time")

			// TODO: when implementation is updated, this should be refactored
			// to check for a 404 status code instead of a connection error
			checkRouteError(t, firstGatewayCheckPort, "/", nil, "service one still routable in allotted time")

			assert.NoError(t, resources.Delete(ctx, firstGateway))
			assert.NoError(t, resources.Delete(ctx, secondGateway))

			return ctx
		})

	testenv.Test(t, feature.Feature())
}

func gatewayStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gwv1beta1.Gateway{}
		if err := resources.Get(ctx, gatewayName, namespace, updated); err != nil {
			return false
		}

		return checkFn(updated.Status.Conditions)
	}
}

func deploymentReplicasSetAsExpected(ctx context.Context, resources *resources.Resources, gatewayName, namespace string, expectedReplicas int32) func() bool {
	return func() bool {
		deployment := &apps.Deployment{}
		if err := resources.Get(ctx, gatewayName, namespace, deployment); err != nil {
			return false
		}

		if deployment.Spec.Replicas == nil {
			return false
		}

		return *deployment.Spec.Replicas == expectedReplicas
	}
}

func gatewayClassStatusCheck(ctx context.Context, resources *resources.Resources, gatewayClassName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gwv1beta1.GatewayClass{}
		if err := resources.Get(ctx, gatewayClassName, namespace, updated); err != nil {
			return false
		}

		return checkFn(updated.Status.Conditions)
	}
}

func listenerStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, namespace string, checkFn func(gwv1beta1.ListenerStatus) bool) func() bool {
	return func() bool {
		updated := &gwv1beta1.Gateway{}
		if err := resources.Get(ctx, gatewayName, namespace, updated); err != nil {
			return false
		}

		for _, listener := range updated.Status.Listeners {
			if ok := checkFn(listener); ok {
				return true
			}
		}

		return false
	}
}

func httpRouteStatusCheck(ctx context.Context, resources *resources.Resources, gatewayName, routeName, namespace string, checkFn func([]meta.Condition) bool) func() bool {
	return func() bool {
		updated := &gwv1alpha2.HTTPRoute{}
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
		updated := &gwv1alpha2.TCPRoute{}
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

func createListenerStatusConditionsCheck(expected []meta.Condition) func(gwv1beta1.ListenerStatus) bool {
	return createListenerStatusConditionsFnCheck(createConditionsCheck(expected))
}

func createListenerStatusConditionsFnCheck(checkFn func([]meta.Condition) bool) func(gwv1beta1.ListenerStatus) bool {
	return func(actual gwv1beta1.ListenerStatus) bool {
		return checkFn(actual.Conditions)
	}
}

func listenerAttachedRoutes(expectedRoutes int32, listenerNames ...string) func(gwv1beta1.ListenerStatus) bool {
	return func(actual gwv1beta1.ListenerStatus) bool {
		// Allow optionally specifying a specific listener name
		if len(listenerNames) > 0 && !slices.Contains(listenerNames, string(actual.Name)) {
			return false
		}

		if actual.AttachedRoutes == expectedRoutes {
			return true
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

func conditionAccepted(conditions []meta.Condition) bool {
	return createConditionsCheck([]meta.Condition{
		{Type: "Accepted", Status: "True"},
	})(conditions)
}

func conditionReady(conditions []meta.Condition) bool {
	return createConditionsCheck([]meta.Condition{
		{Type: "Ready", Status: "True"},
	})(conditions)
}

func conditionInSync(conditions []meta.Condition) bool {
	return createConditionsCheck([]meta.Condition{
		{Type: "InSync", Status: "True"},
	})(conditions)
}

func createHTTPSListener(ctx context.Context, t *testing.T, port gwv1beta1.PortNumber) gwv1beta1.Listener {
	t.Helper()

	namespace := e2e.Namespace(ctx)
	gatewayNamespace := gwv1beta1.Namespace(namespace)

	return gwv1beta1.Listener{
		Name:     "https",
		Port:     port,
		Protocol: gwv1beta1.HTTPSProtocolType,
		TLS: &gwv1beta1.GatewayTLSConfig{
			CertificateRefs: []gwv1beta1.SecretObjectReference{{
				Name:      "consul-server-cert",
				Namespace: &gatewayNamespace,
			}},
		},
	}
}

func createHTTPListener(ctx context.Context, t *testing.T, port gwv1beta1.PortNumber) gwv1beta1.Listener {
	t.Helper()

	return gwv1beta1.Listener{
		Name:     "http",
		Port:     port,
		Protocol: gwv1beta1.HTTPProtocolType,
	}
}

func createGateway(ctx context.Context, t *testing.T, resources *resources.Resources, gatewayName, gatewayNamespace string, gc *gwv1beta1.GatewayClass, listeners []gwv1beta1.Listener) *gwv1beta1.Gateway {
	t.Helper()

	gw := &gwv1beta1.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      gatewayName,
			Namespace: gatewayNamespace,
		},
		Spec: gwv1beta1.GatewaySpec{
			GatewayClassName: gwv1beta1.ObjectName(gc.Name),
			Listeners:        listeners,
		},
	}

	err := resources.Create(ctx, gw)
	require.NoError(t, err)

	return gw
}

type GatewayClassConfigParams struct {
	UseHostPorts     *bool
	DefaultInstances *int32
	MinInstances     *int32
	MaxInstances     *int32
}

func createGatewayClass(ctx context.Context, t *testing.T, resources *resources.Resources) (*apigwv1alpha1.GatewayClassConfig, *gwv1beta1.GatewayClass) {
	return createGatewayClassWithParams(ctx, t, resources, GatewayClassConfigParams{})
}

func createGatewayClassWithParams(ctx context.Context, t *testing.T, resources *resources.Resources, params GatewayClassConfigParams) (*apigwv1alpha1.GatewayClassConfig, *gwv1beta1.GatewayClass) {
	t.Helper()

	// Expose ports on the Docker host
	// This will cause resource conflicts preventing a gateway from becoming ready
	// if gateway listeners do not attempt to bind to a unique port
	useHostPorts := true
	if params.UseHostPorts != nil {
		useHostPorts = *params.UseHostPorts
	}

	// Override default instances if specified in params
	var defaultInstances int32 = 1
	if params.DefaultInstances != nil {
		defaultInstances = *params.DefaultInstances
	}

	namespace := e2e.Namespace(ctx)
	configName := envconf.RandomName("gcc", 16)
	className := envconf.RandomName("gc", 16)

	gcc := &apigwv1alpha1.GatewayClassConfig{
		ObjectMeta: meta.ObjectMeta{
			Name:      configName,
			Namespace: namespace,
		},
		Spec: apigwv1alpha1.GatewayClassConfigSpec{
			ImageSpec: apigwv1alpha1.ImageSpec{
				ConsulAPIGateway: e2e.DockerImage(ctx),
				Envoy:            e2e.EnvoyImage,
			},
			ServiceType:  serviceType(core.ServiceTypeNodePort),
			UseHostPorts: useHostPorts,
			LogLevel:     "trace",
			DeploymentSpec: apigwv1alpha1.DeploymentSpec{
				DefaultInstances: &defaultInstances,
				MaxInstances:     params.MaxInstances,
				MinInstances:     params.MinInstances,
			},
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

	gc := &gwv1beta1.GatewayClass{
		ObjectMeta: meta.ObjectMeta{
			Name: className,
		},
		Spec: gwv1beta1.GatewayClassSpec{
			ControllerName: k8s.ControllerName,
			ParametersRef: &gwv1beta1.ParametersReference{
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

func createTCPRoute(ctx context.Context, t *testing.T, resources *resources.Resources, namespace string, gatewayName string, listenerName gwv1alpha2.SectionName, routeName string, serviceName string, port gwv1alpha2.PortNumber) {
	t.Helper()

	route := &gwv1alpha2.TCPRoute{
		ObjectMeta: meta.ObjectMeta{
			Name:      routeName,
			Namespace: namespace,
		},
		Spec: gwv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1alpha2.ParentReference{{
					Name:        gwv1alpha2.ObjectName(gatewayName),
					SectionName: &listenerName,
				}},
			},
			Rules: []gwv1alpha2.TCPRouteRule{{
				BackendRefs: []gwv1alpha2.BackendRef{{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Name: gwv1alpha2.ObjectName(serviceName),
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
func checkGatewayConfigAnnotation(ctx context.Context, t *testing.T, resources *resources.Resources, gatewayName, namespace string, gcc *apigwv1alpha1.GatewayClassConfig) {
	t.Helper()

	expectedCfg, err := json.Marshal(gcc.Spec)
	require.NoError(t, err)

	gw := &gwv1beta1.Gateway{}
	require.Eventually(t, func() bool {
		err := resources.Get(ctx, gatewayName, namespace, gw)
		return err == nil
	}, checkTimeout, checkInterval, "no gateway found in the allotted time")

	actualCfg, ok := gw.Annotations[`api-gateway.consul.hashicorp.com/config`]
	assert.True(t, ok)
	assert.Equal(t, string(expectedCfg), actualCfg)
}

func checkCrossNamespacePolicyAppliedToNewNamespace(ctx context.Context, t *testing.T, client consul.Client, namespace string) {
	t.Helper()

	ns, _, err := client.Namespaces().Read(namespace, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ns)
	policyNames := make([]string, 0, len(ns.ACLs.PolicyDefaults))
	for _, acl := range ns.ACLs.PolicyDefaults {
		policyNames = append(policyNames, acl.Name)
	}
	assert.Contains(t, policyNames, "cross-namespace-policy")
}

type httpResponse struct {
	StatusCode int
	Body       string
}

func checkRoute(t *testing.T, port int, path string, expected httpResponse, headers map[string]string, message string) {
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
			t.Log(err)
			return false
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Log(err)
			return false
		}
		t.Log(string(data))

		if resp.StatusCode != expected.StatusCode {
			t.Log("status code", resp.StatusCode)
			return false
		}

		return strings.HasPrefix(string(data), expected.Body)
	}, checkTimeout, checkInterval, message)
}

func checkRouteError(t *testing.T, port int, path string, headers map[string]string, message string) {
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

		_, err = client.Do(req)
		return err != nil
	}, checkTimeout, checkInterval, message)
}

func checkTCPRoute(t *testing.T, port int, expected string, exact bool, message string) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: port,
		})
		if err != nil {
			t.Log(err)
			return false
		}
		data, err := io.ReadAll(conn)
		if err != nil {
			t.Log(err)
			return false
		}
		t.Log(string(data))

		if exact {
			return string(data) == expected
		} else {
			return strings.HasPrefix(string(data), expected)
		}
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
