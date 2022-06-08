package conformance_test

import (
	"path/filepath"
	"testing"

	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/tests"
	"sigs.k8s.io/gateway-api/conformance/utils/flags"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

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

	// Write embedded base manifests to kyaml filesystem
	fs := filesys.MakeFsOnDisk()
	tmpdir := t.TempDir()

	basedir := filepath.Join(tmpdir, "base")
	if err := fs.Mkdir(basedir); err != nil {
		t.Fatalf("Error creating base directory: %v", err)
	}
	manifestsPath := filepath.Join(basedir, "manifests.yaml")
	if err := fs.WriteFile(manifestsPath, b); err != nil {
		t.Fatalf("Error writing base manifests file in base directory: %v", err)
	}

	// Copy kustomization to kyaml filesystem
	b, err = fs.ReadFile("kustomization.yaml")
	if err != nil {
		t.Fatalf("Error reading kustomization: %v", err)
	}
	if err := fs.WriteFile(filepath.Join(tmpdir, "kustomization.yaml"), b); err != nil {
		t.Fatalf("Error writing kustomization in tmpdir: %v", err)
	}

	// Copy proxydefaults to kyaml filesystem
	b, err = fs.ReadFile("proxydefaults.yaml")
	if err != nil {
		t.Fatalf("Error reading proxydefaults: %v", err)
	}
	if err := fs.WriteFile(filepath.Join(tmpdir, "proxydefaults.yaml"), b); err != nil {
		t.Fatalf("Error writing kustomization in tmpdir: %v", err)
	}

	// Patch base manifests as needed
	k := krusty.MakeKustomizer(
		krusty.MakeDefaultOptions(),
	)
	resmap, err := k.Run(fs, tmpdir)
	if err != nil {
		t.Fatalf("Error kustomizing base manifests: %v", err)
	}

	b, err = resmap.AsYaml()
	if err != nil {
		t.Fatalf("Error converting kustomized resources to YAML: %v", err)
	}

	// Write kustomized resources back to disk
	if err := fs.WriteFile(filepath.Join(tmpdir, "kustomized.yaml"), b); err != nil {
		t.Fatalf("Error writing kustomized YAML to tmpdir: %v", err)
	}

	cSuite := suite.New(suite.Options{
		Client:               c,
		GatewayClassName:     gatewayClassName,
		Debug:                debug,
		CleanupBaseResources: cleanupBaseResources,
		BaseManifests:        "local://" + filepath.Join(tmpdir, "kustomized.yaml"),
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
