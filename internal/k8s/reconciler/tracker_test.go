package reconciler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestStatusTracker(t *testing.T) {
	t.Parallel()

	type gatewayDeployment struct {
		name types.NamespacedName
		pod  *core.Pod
	}

	initialDeployTime := meta.Now()
	initialDeployment := &gatewayDeployment{
		name: types.NamespacedName{
			Name:      "pod",
			Namespace: "default",
		},
		pod: &core.Pod{
			ObjectMeta: meta.ObjectMeta{
				Generation:        0,
				CreationTimestamp: initialDeployTime,
			},
		},
	}
	currentDeployment := &gatewayDeployment{
		name: types.NamespacedName{
			Name:      "pod",
			Namespace: "default",
		},
		pod: &core.Pod{
			ObjectMeta: meta.ObjectMeta{
				Generation:        1,
				CreationTimestamp: initialDeployTime,
			},
		},
	}

	laterDeployment := &gatewayDeployment{
		name: types.NamespacedName{
			Name:      "pod",
			Namespace: "default",
		},
		pod: &core.Pod{
			ObjectMeta: meta.ObjectMeta{
				Generation:        1,
				CreationTimestamp: meta.NewTime(initialDeployTime.Time.Add(1 * time.Hour)),
			},
		},
	}

	untrackedDeployment := &gatewayDeployment{
		name: types.NamespacedName{
			Name:      "untracked",
			Namespace: "default",
		},
		pod: &core.Pod{
			ObjectMeta: meta.ObjectMeta{
				Generation:        1,
				CreationTimestamp: meta.NewTime(initialDeployTime.Time.Add(1 * time.Hour)),
			},
		},
	}

	tracker := NewStatusTracker()
	// not found
	condition := testCondition()
	updated := tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.True(t, updated)

	// same conditions - cached
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.False(t, updated)

	// check condition Types
	condition.Type = "new type"
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.True(t, updated)

	// check condition Reason
	condition.Reason = "new reason"
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.True(t, updated)

	// check condition Status
	condition.Status = meta.ConditionUnknown
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.True(t, updated)

	// check condition lengths
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition, testCondition()})
	require.True(t, updated)

	// check pod generation
	condition = testCondition()
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.True(t, updated)
	updated = tracker.UpdateStatus(initialDeployment.name, initialDeployment.pod, []meta.Condition{condition, condition})
	require.False(t, updated)

	// check pod timestamp
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.False(t, updated)
	updated = tracker.UpdateStatus(laterDeployment.name, laterDeployment.pod, []meta.Condition{condition})
	require.True(t, updated)
	updated = tracker.UpdateStatus(currentDeployment.name, currentDeployment.pod, []meta.Condition{condition})
	require.False(t, updated)

	// check old delete
	require.Len(t, tracker.statuses, 1)
	tracker.DeleteStatus(untrackedDeployment.name)
	require.Len(t, tracker.statuses, 1)
	tracker.DeleteStatus(currentDeployment.name)
	require.Len(t, tracker.statuses, 0)
}

func testCondition() meta.Condition {
	return meta.Condition{
		Type:   string(gateway.GatewayConditionScheduled),
		Reason: string(gateway.GatewayReasonNotReconciled),
		Status: meta.ConditionFalse,
	}
}
