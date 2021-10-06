package utils

import (
	"errors"
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestRescheduleSyncError(t *testing.T) {
	inner := errors.New("test")

	result, err := RescheduleSyncError(inner)
	require.Error(t, err)
	require.Equal(t, ctrl.Result{}, result)

	result, err = RescheduleSyncError(core.NewSyncError(inner))
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, result)
}
