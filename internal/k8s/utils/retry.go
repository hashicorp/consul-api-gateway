package utils

import (
	"errors"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	requeueAfter = 100 * time.Millisecond
)

// RescheduleSyncError allows us to reschedule a synchronization
// error while not blocking the reconciliation loop because of
// immediate rescheduling
func RescheduleSyncError(err error) (ctrl.Result, error) {
	var syncErr core.SyncError
	if errors.As(err, &syncErr) {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, err
}
