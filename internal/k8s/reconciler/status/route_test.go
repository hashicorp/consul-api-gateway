// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package status

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	rerrors "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
)

func TestRouteFilterParentStatuses(t *testing.T) {
	t.Parallel()

	status := gwv1alpha2.RouteStatus{
		Parents: []gwv1alpha2.RouteParentStatus{{
			ParentRef: gwv1alpha2.ParentReference{
				Name: "expected",
			},
			ControllerName: "expected",
		}, {
			ParentRef: gwv1alpha2.ParentReference{
				Name: "expected",
			},
			ControllerName: "other",
		}, {
			ParentRef: gwv1alpha2.ParentReference{
				Name: "other",
			},
			ControllerName: "other",
		}},
	}

	statuses := filterParentStatuses(status, "expected")
	require.Len(t, statuses, 2)
	require.Equal(t, "expected", string(statuses[0].ParentRef.Name))
	require.Equal(t, "other", string(statuses[0].ControllerName))
	require.Equal(t, "other", string(statuses[1].ParentRef.Name))
	require.Equal(t, "other", string(statuses[1].ControllerName))
}

func TestRouteMergedStatusAndBinding(t *testing.T) {
	t.Parallel()

	parentRef := gwv1alpha2.ParentReference{
		Name: "expected",
	}
	refID := common.AsJSON(parentRef)

	statuses := make(RouteStatuses)
	status := gwv1alpha2.RouteStatus{
		Parents: []gwv1alpha2.RouteParentStatus{{
			ParentRef:      parentRef,
			ControllerName: "expected",
		}, {
			ParentRef:      parentRef,
			ControllerName: "other",
		}, {
			ParentRef: gwv1alpha2.ParentReference{
				Name: "other",
			},
			ControllerName: "other",
		}},
	}

	statuses.Bound(refID)
	merged := statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 3)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "expected", string(merged[0].ControllerName))
	require.Len(t, merged[0].Conditions, 2)
	require.Equal(t, "Route accepted.", merged[0].Conditions[0].Message)
	require.Equal(t, "expected", string(merged[1].ParentRef.Name))
	require.Equal(t, "other", string(merged[1].ControllerName))
	require.Equal(t, "other", string(merged[2].ParentRef.Name))
	require.Equal(t, "other", string(merged[2].ControllerName))

	statuses.BindFailed(service.NewResolutionErrors(), errors.New("expected"), refID)
	merged = statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 3)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "expected", string(merged[0].ControllerName))
	require.Equal(t, "expected", merged[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonBindError, merged[0].Conditions[0].Reason)

	statuses.BindFailed(service.NewResolutionErrors(), rerrors.NewBindErrorHostnameMismatch("expected"), refID)
	merged = statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 3)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "expected", string(merged[0].ControllerName))
	require.Equal(t, "expected", merged[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonListenerHostnameMismatch, merged[0].Conditions[0].Reason)

	statuses.BindFailed(service.NewResolutionErrors(), rerrors.NewBindErrorListenerNamespacePolicy("expected"), refID)
	merged = statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 3)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "expected", string(merged[0].ControllerName))
	require.Equal(t, "expected", merged[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonListenerNamespacePolicy, merged[0].Conditions[0].Reason)

	statuses.BindFailed(service.NewResolutionErrors(), rerrors.NewBindErrorRouteKind("expected"), refID)
	merged = statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 3)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "expected", string(merged[0].ControllerName))
	require.Equal(t, "expected", merged[0].Conditions[0].Message)
	require.Equal(t, RouteConditionReasonInvalidRouteKind, merged[0].Conditions[0].Reason)

	statuses.Bound(refID)
	merged = statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 3)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "expected", string(merged[0].ControllerName))
	require.Equal(t, "Route accepted.", merged[0].Conditions[0].Message)

	statuses.Remove(refID)
	merged = statuses.mergedStatus(status, "expected", 0).Parents
	require.Len(t, merged, 2)
	require.Equal(t, "expected", string(merged[0].ParentRef.Name))
	require.Equal(t, "other", string(merged[0].ControllerName))
	require.Equal(t, "other", string(merged[1].ParentRef.Name))
	require.Equal(t, "other", string(merged[1].ControllerName))
}

func TestNeedsUpdate(t *testing.T) {
	t.Parallel()

	parentRef := gwv1alpha2.ParentReference{
		Name: "expected",
	}
	refID := common.AsJSON(parentRef)

	statuses := make(RouteStatuses)
	status := gwv1alpha2.RouteStatus{
		Parents: []gwv1alpha2.RouteParentStatus{{
			ParentRef:      parentRef,
			ControllerName: "expected",
		}, {
			ParentRef:      parentRef,
			ControllerName: "other",
		}, {
			ParentRef: gwv1alpha2.ParentReference{
				Name: "other",
			},
			ControllerName: "other",
		}},
	}

	status = statuses.mergedStatus(status, "expected", 0)
	_, needsUpdate := statuses.NeedsUpdate(status, "expected", 0)
	require.False(t, needsUpdate)

	statuses.Bound(refID)
	_, needsUpdate = statuses.NeedsUpdate(status, "expected", 0)
	require.True(t, needsUpdate)

	status = statuses.mergedStatus(status, "expected", 0)
	_, needsUpdate = statuses.NeedsUpdate(status, "expected", 0)
	require.False(t, needsUpdate)
}
