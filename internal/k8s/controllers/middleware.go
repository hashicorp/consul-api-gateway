package controllers

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	requeueAfter = 100 * time.Millisecond
)

// NewSyncRequeuingMiddleware handles delaying requeues in the case of synchronization
// errors in order to not block the reconciliation loop
func NewSyncRequeueingMiddleware(reconciler reconcile.Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
		return RescheduleSyncError(reconciler.Reconcile(ctx, req))
	})
}

// RescheduleSyncError allows us to reschedule a synchronization
// error while not blocking the reconciliation loop because of
// immediate rescheduling
func RescheduleSyncError(result ctrl.Result, err error) (ctrl.Result, error) {
	if err == nil {
		return result, nil
	}

	var syncErr core.SyncError
	if errors.As(err, &syncErr) {
		// clobber the result that was passed since it'll be
		// ignored anyway because of the returned error
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return result, err
}
