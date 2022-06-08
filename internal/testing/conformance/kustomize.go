package conformance

import (
	"fmt"
	"path/filepath"
	"testing"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func kustomizeBaseManifests(t *testing.T, baseManifests []byte) (string, error) {
	// Create kyaml wrapper for local filesystem, needed for Kustomizer.Run, and tmpdir
	fs := filesys.MakeFsOnDisk()
	tmpdir := t.TempDir()

	// Write embedded base manifests to kyaml filesystem
	basedir := filepath.Join(tmpdir, "base")
	if err := fs.Mkdir(basedir); err != nil {
		return "", fmt.Errorf("Error creating base directory: %v", err)
	}
	if err := fs.WriteFile(filepath.Join(basedir, "manifests.yaml"), baseManifests); err != nil {
		return "", fmt.Errorf("Error writing base manifests file in base directory: %v", err)
	}

	// Copy kustomization to tmpdir
	b, err := fs.ReadFile("kustomization.yaml")
	if err != nil {
		return "", fmt.Errorf("Error reading kustomization: %v", err)
	}
	if err := fs.WriteFile(filepath.Join(tmpdir, "kustomization.yaml"), b); err != nil {
		return "", fmt.Errorf("Error writing kustomization in tmpdir: %v", err)
	}

	// Copy proxydefaults to tmpdir
	b, err = fs.ReadFile("proxydefaults.yaml")
	if err != nil {
		return "", fmt.Errorf("Error reading proxydefaults: %v", err)
	}
	if err := fs.WriteFile(filepath.Join(tmpdir, "proxydefaults.yaml"), b); err != nil {
		return "", fmt.Errorf("Error writing kustomization in tmpdir: %v", err)
	}

	// Kustomize base manifests
	k := krusty.MakeKustomizer(
		krusty.MakeDefaultOptions(),
	)
	resmap, err := k.Run(fs, tmpdir)
	if err != nil {
		return "", fmt.Errorf("Error kustomizing base manifests: %v", err)
	}

	// Parse results from kustomization run as YAML
	b, err = resmap.AsYaml()
	if err != nil {
		return "", fmt.Errorf("Error converting kustomized resources to YAML: %v", err)
	}

	// Write kustomized resources back to disk
	kustomizedPath := filepath.Join(tmpdir, "kustomized.yaml")
	if err := fs.WriteFile(kustomizedPath, b); err != nil {
		return "", fmt.Errorf("Error writing kustomized YAML to tmpdir: %v", err)
	}

	return "local://" + kustomizedPath, nil
}
