package conformance

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

//set up conformance test stack

var (
	metalLBManifests = []string{
		"https://raw.githubusercontent.com/metallb/metallb/v0.12.1/manifests/namespace.yaml",
		"https://raw.githubusercontent.com/metallb/metallb/v0.12.1/manifests/metallb.yaml",
	}

	consulApiGatewayCRD = "github.com/hashicorp/consul-api-gateway/config/crd?ref=v0.1.0"
)

const values = `
global:
  name: consul
  image: 'hashicorp/consul:1.11.3'
connectInject:
  enabled: true
controller:
  enabled: true
apiGateway:
  enabled: true
  image: hashicorp/consul-api-gateway:0.1.0`

func kubectlApply(ctx context.Context, path string) error {
	timeoutContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeoutContext, "kubectl", "apply", "-f", path)
	return cmd.Run()
}

func SetUpStack(hostRoute string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		var err error

		log.Println("installing consul api-gateway crds")
		kustomizeCmd := exec.Command("kubectl", "apply", "--kustomize", consulApiGatewayCRD)
		out, err := kustomizeCmd.CombinedOutput()
		if err != nil {
			return nil, errors.Wrap(err, string(out))
		}

		//write and install values yaml
		wdPath, _ := filepath.Abs(".")
		valuesPath := wdPath + "/values.yaml"
		log.Println("helm installing consul")
		err = os.WriteFile(valuesPath, []byte(values), 0644)
		if err != nil {
			return nil, err
		}
		installCmd := exec.Command("helm", "install", "consul", "hashicorp/consul", "--version", "0.41.1", "--values", valuesPath, "--create-namespace", "--namespace", "consul")
		fmt.Println(installCmd)
		out, err = installCmd.CombinedOutput()
		if err != nil {
			return nil, errors.Wrap(err, string(out))
		}

		//install metallb on newly spun up kind cluster
		for _, url := range metalLBManifests {
			err = kubectlApply(ctx, url)
			if err != nil {
				return nil, err
			}
		}

		fmt.Println("set up")

		return ctx, nil
	}
}
