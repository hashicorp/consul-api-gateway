package utils

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureFinalizer ensures that the given finalizer is on the passed object
// it returns a boolean saying whether or not a finalizer was added, and any
// potential errors
func EnsureFinalizer(ctx context.Context, client client.Client, object client.Object, finalizer string) (bool, error) {
	finalizers := object.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return false, nil
		}
	}
	object.SetFinalizers(append(finalizers, finalizer))
	if err := client.Update(ctx, object); err != nil {
		return false, fmt.Errorf("failed to add in-use finalizer: %w", err)
	}
	return true, nil
}

// RemoveFinalizer ensures that the given finalizer is removed from the passed object
// it returns a boolean saying whether or not a finalizer was removed, and any
// potential errors
func RemoveFinalizer(ctx context.Context, client client.Client, object client.Object, finalizer string) (bool, error) {
	finalizers := []string{}
	found := false
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			found = true
			continue
		}
		finalizers = append(finalizers, finalizer)
	}
	if found {
		object.SetFinalizers(finalizers)
		if err := client.Update(ctx, object); err != nil {
			return false, fmt.Errorf("failed to remove in-use finalizer: %w", err)
		}
	}
	return found, nil
}
