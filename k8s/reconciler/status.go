package reconciler

import (
	"context"

	"github.com/cenkalti/backoff"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func updateStatus(ctx context.Context, writer client.StatusWriter, obj client.Object) error {
	return backoff.Retry(func() error {
		return writer.Update(ctx, obj)
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(statusUpdateTimeout), maxStatusUpdateAttempts), ctx))
}
