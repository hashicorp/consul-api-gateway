package gatewayclient

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestRescheduleK8sError(t *testing.T) {
	inner := errors.New("test")

	result, err := RescheduleK8sError(hclog.NewNullLogger(), ctrl.Result{}, inner)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, result)

	result, err = RescheduleK8sError(hclog.NewNullLogger(), ctrl.Result{}, NewK8sError(inner))
	require.Error(t, err)
	require.Equal(t, ctrl.Result{}, result)
}
