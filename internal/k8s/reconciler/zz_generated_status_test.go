package reconciler

// GENERATED from statuses.yaml, DO NOT EDIT DIRECTLY

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayClassAcceptedStatus(t *testing.T) {
	t.Parallel()

	var status GatewayClassAcceptedStatus

	expected := errors.New("expected")

	status = GatewayClassAcceptedStatus{}
	require.Equal(t, "Accepted", status.Condition(0).Message)
	require.Equal(t, GatewayClassConditionReasonAccepted, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = GatewayClassAcceptedStatus{InvalidParameters: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayClassConditionReasonInvalidParameters, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = GatewayClassAcceptedStatus{Waiting: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayClassConditionReasonWaiting, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestGatewayClassStatus(t *testing.T) {
	t.Parallel()

	status := GatewayClassStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = GatewayClassConditionAccepted
	reason = GatewayClassConditionReasonAccepted
	require.Equal(t, conditionType, conditions[0].Type)
	require.Equal(t, reason, conditions[0].Reason)
}

func TestGatewayReadyStatus(t *testing.T) {
	t.Parallel()

	var status GatewayReadyStatus

	expected := errors.New("expected")

	status = GatewayReadyStatus{}
	require.Equal(t, "Ready", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonReady, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = GatewayReadyStatus{ListenersNotValid: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonListenersNotValid, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = GatewayReadyStatus{ListenersNotReady: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonListenersNotReady, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = GatewayReadyStatus{AddressNotAssigned: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonAddressNotAssigned, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestGatewayScheduledStatus(t *testing.T) {
	t.Parallel()

	var status GatewayScheduledStatus

	expected := errors.New("expected")

	status = GatewayScheduledStatus{}
	require.Equal(t, "Scheduled", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonScheduled, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = GatewayScheduledStatus{NotReconciled: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonNotReconciled, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = GatewayScheduledStatus{PodFailed: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonPodFailed, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = GatewayScheduledStatus{Unknown: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonUnknown, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = GatewayScheduledStatus{NoResources: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonNoResources, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestGatewayInSyncStatus(t *testing.T) {
	t.Parallel()

	var status GatewayInSyncStatus

	expected := errors.New("expected")

	status = GatewayInSyncStatus{}
	require.Equal(t, "InSync", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonInSync, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = GatewayInSyncStatus{SyncError: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, GatewayConditionReasonSyncError, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestGatewayStatus(t *testing.T) {
	t.Parallel()

	status := GatewayStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = GatewayConditionReady
	reason = GatewayConditionReasonReady
	require.Equal(t, conditionType, conditions[0].Type)
	require.Equal(t, reason, conditions[0].Reason)

	conditionType = GatewayConditionScheduled
	reason = GatewayConditionReasonScheduled
	require.Equal(t, conditionType, conditions[1].Type)
	require.Equal(t, reason, conditions[1].Reason)

	conditionType = GatewayConditionInSync
	reason = GatewayConditionReasonInSync
	require.Equal(t, conditionType, conditions[2].Type)
	require.Equal(t, reason, conditions[2].Reason)
}

func TestRouteAcceptedStatus(t *testing.T) {
	t.Parallel()

	var status RouteAcceptedStatus

	expected := errors.New("expected")

	status = RouteAcceptedStatus{}
	require.Equal(t, "Route accepted.", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonAccepted, status.Condition(0).Reason)

	status = RouteAcceptedStatus{InvalidRouteKind: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonInvalidRouteKind, status.Condition(0).Reason)

	status = RouteAcceptedStatus{ListenerNamespacePolicy: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonListenerNamespacePolicy, status.Condition(0).Reason)

	status = RouteAcceptedStatus{ListenerHostnameMismatch: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonListenerHostnameMismatch, status.Condition(0).Reason)

	status = RouteAcceptedStatus{BindError: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonBindError, status.Condition(0).Reason)

}

func TestRouteResolvedRefsStatus(t *testing.T) {
	t.Parallel()

	var status RouteResolvedRefsStatus

	expected := errors.New("expected")

	status = RouteResolvedRefsStatus{}
	require.Equal(t, "ResolvedRefs", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonResolvedRefs, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = RouteResolvedRefsStatus{Errors: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonErrors, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = RouteResolvedRefsStatus{ServiceNotFound: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonServiceNotFound, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = RouteResolvedRefsStatus{ConsulServiceNotFound: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonConsulServiceNotFound, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestRouteStatus(t *testing.T) {
	t.Parallel()

	status := RouteStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = RouteConditionAccepted
	reason = RouteConditionReasonAccepted
	require.Equal(t, conditionType, conditions[0].Type)
	require.Equal(t, reason, conditions[0].Reason)

	conditionType = RouteConditionResolvedRefs
	reason = RouteConditionReasonResolvedRefs
	require.Equal(t, conditionType, conditions[1].Type)
	require.Equal(t, reason, conditions[1].Reason)
}

func TestListenerConflictedStatus(t *testing.T) {
	t.Parallel()

	var status ListenerConflictedStatus

	expected := errors.New("expected")

	status = ListenerConflictedStatus{}
	require.Equal(t, "NoConflicts", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonNoConflicts, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = ListenerConflictedStatus{HostnameConflict: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonHostnameConflict, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerConflictedStatus{ProtocolConflict: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonProtocolConflict, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerConflictedStatus{RouteConflict: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonRouteConflict, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestListenerDetachedStatus(t *testing.T) {
	t.Parallel()

	var status ListenerDetachedStatus

	expected := errors.New("expected")

	status = ListenerDetachedStatus{}
	require.Equal(t, "Attached", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonAttached, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = ListenerDetachedStatus{PortUnavailable: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonPortUnavailable, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerDetachedStatus{UnsupportedExtension: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonUnsupportedExtension, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerDetachedStatus{UnsupportedProtocol: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonUnsupportedProtocol, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerDetachedStatus{UnsupportedAddress: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonUnsupportedAddress, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestListenerReadyStatus(t *testing.T) {
	t.Parallel()

	var status ListenerReadyStatus

	expected := errors.New("expected")

	status = ListenerReadyStatus{}
	require.Equal(t, "Ready", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonReady, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = ListenerReadyStatus{Invalid: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonInvalid, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerReadyStatus{Pending: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonPending, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestListenerResolvedRefsStatus(t *testing.T) {
	t.Parallel()

	var status ListenerResolvedRefsStatus

	expected := errors.New("expected")

	status = ListenerResolvedRefsStatus{}
	require.Equal(t, "ResolvedRefs", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonResolvedRefs, status.Condition(0).Reason)
	require.False(t, status.HasError())

	status = ListenerResolvedRefsStatus{InvalidCertificateRef: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonInvalidCertificateRef, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerResolvedRefsStatus{InvalidRouteKinds: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonInvalidRouteKinds, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = ListenerResolvedRefsStatus{RefNotPermitted: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, ListenerConditionReasonRefNotPermitted, status.Condition(0).Reason)
	require.True(t, status.HasError())
}

func TestListenerStatus(t *testing.T) {
	t.Parallel()

	status := ListenerStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = ListenerConditionConflicted
	reason = ListenerConditionReasonNoConflicts
	require.Equal(t, conditionType, conditions[0].Type)
	require.Equal(t, reason, conditions[0].Reason)

	conditionType = ListenerConditionDetached
	reason = ListenerConditionReasonAttached
	require.Equal(t, conditionType, conditions[1].Type)
	require.Equal(t, reason, conditions[1].Reason)

	conditionType = ListenerConditionReady
	reason = ListenerConditionReasonReady
	require.Equal(t, conditionType, conditions[2].Type)
	require.Equal(t, reason, conditions[2].Reason)

	conditionType = ListenerConditionResolvedRefs
	reason = ListenerConditionReasonResolvedRefs
	require.Equal(t, conditionType, conditions[3].Type)
	require.Equal(t, reason, conditions[3].Reason)

	require.True(t, status.Valid())

	validationError := errors.New("error")

	status = ListenerStatus{}
	status.Conflicted.HostnameConflict = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.Conflicted.ProtocolConflict = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.Conflicted.RouteConflict = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.PortUnavailable = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.UnsupportedExtension = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.UnsupportedProtocol = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.UnsupportedAddress = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.ResolvedRefs.InvalidCertificateRef = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.ResolvedRefs.InvalidRouteKinds = validationError
	require.False(t, status.Valid())

	status = ListenerStatus{}
	status.ResolvedRefs.RefNotPermitted = validationError
	require.False(t, status.Valid())
}
