package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func kubectlKustomizeCRDs(ctx context.Context, url string) ([]client.Object, error) {
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
