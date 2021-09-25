package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/k8s"
	"github.com/hashicorp/go-hclog"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
)

func TestServerK8sInitializationError(t *testing.T) {
	t.Parallel()

	var buffer gwTesting.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Output: &buffer,
	})
	require.Equal(t, 1, RunServer(ServerConfig{
		Context:   context.Background(),
		Logger:    logger,
		isTest:    true,
		K8sConfig: k8s.Defaults(),
	}))
	require.Contains(t, buffer.String(), "error initializing the kubernetes secret fetcher")
}
