package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func kubectlKustomizeCRDs(ctx context.Context, url string) ([]*api.CustomResourceDefinition, error) {
	var stdout, stderr bytes.Buffer
	timeoutContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeoutContext, "kubectl", "kustomize", url)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return readCRDs(stdout.Bytes())
}
