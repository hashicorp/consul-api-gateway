package conformance

import (
	"testing"

	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/conformance"
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

	// TODO: Currently failing, need to triage
	tests.HTTPExactPathMatching.ShortName,
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

	// Read embedded base manifests
	b, err := conformance.Manifests.ReadFile("base/manifests.yaml")
	if err != nil {
		t.Fatalf("Error reading embedded base manifests: %v", err)
	}

	// Kustomzie base manifests as needed
	kustomizedBaseManifests, err := kustomizeBaseManifests(t, b)
	if err != nil {
		t.Fatalf("Error kustomizing base manifests: %v", err)
	}

	cSuite := suite.New(suite.Options{
		Client:               c,
		GatewayClassName:     gatewayClassName,
		Debug:                debug,
		CleanupBaseResources: cleanupBaseResources,
		BaseManifests:        kustomizedBaseManifests,
		SupportedFeatures:    []suite.SupportedFeature{},
	})
	cSuite.Setup(t)

	var testsToRun []suite.ConformanceTest
	for _, conformanceTest := range tests.ConformanceTests {
		if !slices.Contains(testsToSkip, conformanceTest.ShortName) {
			testsToRun = append(testsToRun, conformanceTest)
		}
	}
	cSuite.Run(t, testsToRun)
}
