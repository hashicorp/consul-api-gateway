package gatewayclient

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/go-hclog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	requeueAfter = 100 * time.Millisecond
)

// NewSyncRequeuingMiddleware handles delaying requeues in the case of synchronization
// errors in order to not block the reconciliation loop
func NewRequeueingMiddleware(logger hclog.Logger, reconciler reconcile.Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
		result, err := reconciler.Reconcile(ctx, req)
		return RescheduleK8sError(logger, result, err)
	})
}

// RescheduleK8sError allows us to reschedule all non-Kubernetes
// errors while not blocking the reconciliation loop because of
// immediate rescheduling
func RescheduleK8sError(logger hclog.Logger, result ctrl.Result, err error) (ctrl.Result, error) {
	if err == nil {
		return result, nil
	}

	var k8sErr K8sError
	if errors.As(err, &k8sErr) {
		return result, err
	}

	logger.Warn("received non-Kubernetes error, delaying requeue", "error", err)

	// clobber the result that was passed since it'll be
	// ignored anyway because of the returned error
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
