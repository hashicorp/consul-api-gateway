package conformance

import (
	"context"
	"os/exec"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/testing/e2e"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

//set up conformance test stack

var (
	metalLBManifests = []string{
		"https://raw.githubusercontent.com/metallb/metallb/v0.12.1/manifests/namespace.yaml",
		"https://raw.githubusercontent.com/metallb/metallb/v0.12.1/manifests/metallb.yaml",
	}
)

func kubectlApply(ctx context.Context, path string) error {
	timeoutContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeoutContext, "kubectl", "apply", "-f", path)
	return cmd.Run()
}

func SetUpStack(hostRoute string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		var err error
		//invoke e2e stack
		ctx, err = e2e.SetUpStack(hostRoute)(ctx, cfg)
		if err != nil {
			return nil, err
		}

		//install metallb on newly spun up kind cluster
		for _, url := range metalLBManifests {
			err = kubectlApply(ctx, url)
			if err != nil {
				return nil, err
			}
		}
		return ctx, nil
	}
}
