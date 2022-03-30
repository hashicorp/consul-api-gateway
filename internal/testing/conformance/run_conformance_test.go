//go:build conformance
// +build conformance

package conformance

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	cp "github.com/nmrshll/go-cp"
	"github.com/pkg/errors"
	"sigs.k8s.io/e2e-framework/pkg/env"
)

//This file is intended to help facilitate an easy and local way of running the
//conformance tests defined here https://github.com/kubernetes-sigs/gateway-api/blob/master/conformance/conformance_test.go

//This is a work in progress

var (
	testenv   env.Environment
	hostRoute string
)

const (
	ConformanceTestRepository = "https://github.com/kubernetes-sigs/gateway-api.git"
	ConformanceTestPath       = "conformance/conformance_test.go"
)

func TestMain(m *testing.M) {
	//set up test env
	hostRoute := os.Getenv("DOCKER_HOST_ROUTE")
	if hostRoute == "" {
		hostRoute = "host.docker.internal"
	}

	testenv = env.New()
	testenv.Setup(
		SetUpStack(hostRoute),
	)

	// testenv.Finish(
	// 	e2e.TearDownStack,
	// )

	testenv.Run(m)

}

func TestRunConformanceTestSuite(t *testing.T) {
	//TODO wait for gateway to be ready

	//TODO invoke conformance tests
	err := fetchConformanceTests(context.Background())
	if err != nil {
		t.Error(err)
	}

}

func fetchConformanceTests(ctx context.Context) error {
	fmt.Println(filepath.Abs("."))
	path, _ := filepath.Abs(".")
	repoPath := path + "/gateway-api"

	//clone gateway-api repository
	log.Printf("Cloning %s into %s...\n", ConformanceTestRepository, repoPath)

	_, err := git.PlainClone(repoPath, false, &git.CloneOptions{
		URL:      ConformanceTestRepository,
		Progress: os.Stdout,
		Depth:    1,
	})

	log.Println("copying kustomize.yaml into cloned repository...")
	// cp kustomization.yaml proxydefaults.yaml gateway-api/conformance/
	err = cp.CopyFile(path+"/kustomization.yaml", path+"/gateway-api/conformance/kustomization.yaml")
	if err != nil {
		return err
	}
	err = cp.CopyFile(path+"/proxydefaults.yaml", path+"/gateway-api/conformance/proxydefaults.yaml")
	if err != nil {
		return err
	}

	log.Println("installing dependencies...")

	getCmd := exec.Command("go", "get", "./...")
	getCmd.Dir = repoPath
	out, err := getCmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}
	fmt.Println(string(out))

	log.Println("kustomize test specific kubernetes objects...")
	kustomizeCmd := exec.Command("kubectl", "kustomize", "./", "--output", "./base/manifests.yaml")
	kustomizeCmd.Dir = repoPath + "/conformance"
	out, err = kustomizeCmd.CombinedOutput()

	fmt.Println(string(out))

	log.Println("applying test specific kubernetes objects...")

	applyCmd := exec.Command("kubectl", "apply", "-f", "base/manifests.yaml")
	applyCmd.Dir = repoPath + "/conformance"
	out, err = applyCmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	fmt.Println(string(out))

	fmt.Println(string(out))

	testCmd := exec.Command("go", "test", ConformanceTestPath, "--gateway-class", "consul-api-gateway", "--cleanup", "0")
	testCmd.Dir = repoPath
	fmt.Println(testCmd)

	out, err = testCmd.CombinedOutput()
	fmt.Println(string(out))

	if err != nil {
		return errors.Wrap(err, string(out))
	}

	fmt.Println(string(out))

	fmt.Println(os.Remove(repoPath))
	return err
}
