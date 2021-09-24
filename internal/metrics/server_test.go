package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestServerShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	errs := make(chan error, 1)
	go func() {
		errs <- RunServer(ctx, hclog.NewNullLogger(), "127.0.0.1:0")
	}()

	require.NoError(t, <-errs)
}
