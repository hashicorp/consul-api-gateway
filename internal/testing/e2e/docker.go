package e2e

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/google/uuid"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type dockerImageContext struct{}

var dockerImageContextKey = dockerImageContext{}

const (
	envvarExtraDockerImages = envvarPrefix + "DOCKER_IMAGES"
)

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
	r, err := dockerClient.ImageBuild(ctx, tar, types.ImageBuildOptions{
		Dockerfile: "Dockerfile.local",
		Tags:       []string{tag},
		Remove:     true,
	})
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	log.Print(string(b))

	return context.WithValue(ctx, dockerImageContextKey, tag), nil
}

func DockerImage(ctx context.Context) string {
	image := ctx.Value(dockerImageContextKey)
	if image == nil {
		panic("must run this with an integration test that has called BuildDockerImage")
	}
	return image.(string)
}

func ExtraDockerImages() []string {
	images := os.Getenv(envvarExtraDockerImages)
	if images != "" {
		return strings.Split(images, ",")
	}
	return nil
}
