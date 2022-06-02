package conformance_test

import (
	"context"
	"os/exec"
	"testing"

	apps "k8s.io/api/apps/v1"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/conformance/tests"
	"sigs.k8s.io/gateway-api/conformance/utils/flags"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	cleanupBaseResources = false
	debug                = false
	gatewayClassName     = "consul-api-gateway"
)

var testsToSkip = []string{
	// Test asserts 404 response which we can't yet provide due to xDS control
	tests.HTTPRouteHeaderMatching.ShortName,

	// FIXME: Tests create a gateway which gets stuck in status Unknown, with
	// reason NotReconciled, "Waiting for controller" (why?)
	tests.HTTPRouteListenerHostnameMatching.ShortName,
	tests.HTTPRouteDisallowedKind.ShortName,
}

func TestConformance(t *testing.T) {
	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Error loading Kubernetes config: %v", err)
	}

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		t.Fatalf("Error initializing Kubernetes client: %v", err)
	}
	v1alpha2.AddToScheme(c.Scheme())

	t.Logf("Running conformance tests with %s GatewayClass", *flags.GatewayClassName)

	cSuite := suite.New(suite.Options{
		Client:               c,
		GatewayClassName:     gatewayClassName,
		Debug:                debug,
		CleanupBaseResources: cleanupBaseResources,
		SupportedFeatures:    []suite.SupportedFeature{},
	})
	cSuite.Setup(t)

	// Update conformance test infra resources as needed
	deployments := &apps.DeploymentList{}
	if err := c.List(context.Background(), deployments); err != nil {
		t.Fatalf("Error fetching deployments: %v", err)
	}
	for _, d := range deployments.Items {
		// Add connect-inject annotation to each Deployment. This is required due to
		// containerPort not being defined on Deployments upstream. Though containerPort
		// is optional, Consul relies on it as a default value in the absence of a
		// connect-service-port annotation.
		d.Annotations["consul.hashicorp.com/connect-service-port"] = "3000"

		// We don't have enough resources in the GitHub-hosted Actions runner to support 2 replicas
		var numReplicas int32 = 1
		d.Spec.Replicas = &numReplicas

		if err := c.Update(context.Background(), &d); err != nil {
			t.Fatalf("Error updating deployment: %v", err)
		}
	}

	if err = exec.Command("kubectl", "apply", "-f", "proxydefaults.yaml").Run(); err != nil {
		t.Fatalf("Error creating ProxyDefaults: %v", err)
	}

	var testsToRun []suite.ConformanceTest
	for _, conformanceTest := range tests.ConformanceTests {
		if !slices.Contains(testsToSkip, conformanceTest.ShortName) {
			testsToRun = append(testsToRun, conformanceTest)
		}
	}
	cSuite.Run(t, testsToRun)
}
