package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestPodTracker(t *testing.T) {
	initialDeployTime := meta.Now()
	initialPod := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:              "pod",
			Namespace:         "default",
			Generation:        0,
			CreationTimestamp: initialDeployTime,
		},
	}
	currentPod := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:              "pod",
			Namespace:         "default",
			Generation:        1,
			CreationTimestamp: initialDeployTime,
		},
	}

	laterPod := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:       "pod",
			Namespace:  "default",
			Generation: 1,
			CreationTimestamp: meta.Time{
				Time: initialDeployTime.Add(1 * time.Hour),
			},
		},
	}

	untrackedPod := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:       "untracked",
			Namespace:  "default",
			Generation: 1,
		},
	}

	tracker := NewPodTracker()
	// not found
	condition := testCondition()
	updated := tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.True(t, updated)

	// same conditions - cached
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.False(t, updated)

	// check condition Types
	condition.Type = "new type"
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.True(t, updated)

	// check condition Reason
	condition.Reason = "new reason"
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.True(t, updated)

	// check condition Status
	condition.Status = meta.ConditionUnknown
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.True(t, updated)

	// check condition lengths
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition, testCondition()})
	require.True(t, updated)

	// check pod generation
	condition = testCondition()
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.True(t, updated)
	updated = tracker.UpdateStatus(initialPod, []meta.Condition{condition, condition})
	require.False(t, updated)

	// check pod timestamp
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.False(t, updated)
	updated = tracker.UpdateStatus(laterPod, []meta.Condition{condition})
	require.True(t, updated)
	updated = tracker.UpdateStatus(currentPod, []meta.Condition{condition})
	require.False(t, updated)

	// check old delete
	require.Len(t, tracker.statuses, 1)
	tracker.DeleteStatus(untrackedPod)
	require.Len(t, tracker.statuses, 1)
	tracker.DeleteStatus(currentPod)
	require.Len(t, tracker.statuses, 1)
	tracker.DeleteStatus(laterPod)
	require.Len(t, tracker.statuses, 0)
}

func testCondition() meta.Condition {
	return meta.Condition{
		Type:   string(gateway.GatewayConditionScheduled),
		Reason: string(gateway.GatewayReasonNotReconciled),
		Status: meta.ConditionFalse,
	}
}
