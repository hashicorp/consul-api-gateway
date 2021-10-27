package e2e

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path"
	"time"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func CrossCompileProject(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Cross compiling consul-api-gateway")

	var stdout, stderr bytes.Buffer
	timeoutContext, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	rootProjectPath := path.Join("..", "..", "..")
	cmd := exec.CommandContext(timeoutContext, "go", "build", "-o", path.Join(rootProjectPath, "consul-api-gateway"), rootProjectPath)
	cmd.Env = append(os.Environ(), "GOOS=linux")
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, errors.New(stderr.String())
	}
	return ctx, nil
}
