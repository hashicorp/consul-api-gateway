package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestMapGatewayConditionsFromPod(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name               string
		pod                *core.Pod
		expectedConditions []meta.Condition
	}{{
		name: "initial",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodPending,
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonNotReconciled),
			Status: meta.ConditionFalse,
		}},
	}, {
		name: "unknown",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodUnknown,
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonNotReconciled),
			Status: meta.ConditionFalse,
		}},
	}, {
		name: "succeeded",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodSucceeded,
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonNoResources),
			Status: meta.ConditionFalse,
		}},
	}, {
		name: "failed",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodFailed,
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonNoResources),
			Status: meta.ConditionFalse,
		}},
	}, {
		name: "pending-unschedulable",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodPending,
				Conditions: []core.PodCondition{{
					Type:   core.PodScheduled,
					Status: core.ConditionFalse,
					Reason: "Unschedulable 0/1 nodes are available: 1 node(s) didn't have free ports for the requested pod ports.",
				}},
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonNoResources),
			Status: meta.ConditionFalse,
		}},
	}, {
		name: "pending-scheduled",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodPending,
				Conditions: []core.PodCondition{{
					Type:   core.PodScheduled,
					Status: core.ConditionTrue,
				}},
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonScheduled),
			Status: meta.ConditionTrue,
		}},
	}, {
		name: "running-initial",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodRunning,
				Conditions: []core.PodCondition{{
					Type:   core.PodReady,
					Status: core.ConditionFalse,
				}},
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonScheduled),
			Status: meta.ConditionTrue,
		}, {
			Type:   string(gateway.GatewayConditionReady),
			Reason: string(gateway.GatewayReasonListenersNotReady),
			Status: meta.ConditionFalse,
		}},
	}, {
		name: "running-ready",
		pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodRunning,
				Conditions: []core.PodCondition{{
					Type:   core.PodReady,
					Status: core.ConditionTrue,
				}},
			},
		},
		expectedConditions: []meta.Condition{{
			Type:   string(gateway.GatewayConditionScheduled),
			Reason: string(gateway.GatewayReasonScheduled),
			Status: meta.ConditionTrue,
		}, {
			Type:   string(gateway.GatewayConditionReady),
			Reason: string(gateway.GatewayReasonReady),
			Status: meta.ConditionTrue,
		}},
	}} {
		t.Run(test.name, func(t *testing.T) {
			conditions := MapGatewayConditionsFromPod(test.pod)

			require.Len(t, conditions, len(test.expectedConditions))
			for i, expected := range test.expectedConditions {
				actual := conditions[i]
				require.Equal(t, expected.Type, actual.Type)
				require.Equal(t, expected.Status, actual.Status)
				require.Equal(t, expected.Reason, actual.Reason)
			}
		})
	}
}
