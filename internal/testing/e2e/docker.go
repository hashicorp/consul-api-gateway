package e2e

import (
	"context"
	"fmt"
	"log"
	"path"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/google/uuid"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type dockerImageContext struct{}

var dockerImageContextKey = dockerImageContext{}

func BuildDockerImage(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Building docker image")

	tag := fmt.Sprintf("consul-api-gateway:%s", uuid.New().String())
	dockerClient, err := client.NewClientWithOpts()
	if err != nil {
		return nil, err
	}
	tar, err := archive.TarWithOptions(path.Join("..", "..", ".."), &archive.TarOptions{})
	if err != nil {
		return nil, err
	}
	_, err = dockerClient.ImageBuild(ctx, tar, types.ImageBuildOptions{
		Dockerfile: "Dockerfile.local",
		Tags:       []string{tag},
		Remove:     true,
	})
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, dockerImageContextKey, tag), nil
}

func DockerImage(ctx context.Context) string {
	image := ctx.Value(dockerImageContextKey)
	if image == nil {
		panic("must run this with an integration test that has called BuildDockerImage")
	}
	return image.(string)
}
