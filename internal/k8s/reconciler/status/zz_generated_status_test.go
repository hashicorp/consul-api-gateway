package status

// GENERATED from statuses.yaml, DO NOT EDIT DIRECTLY

import (
	"encoding/json"
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

func TestGatewayClassAcceptedStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := GatewayClassAcceptedStatus{
		InvalidParameters: errors.New("InvalidParameters"),
		Waiting:           errors.New("Waiting"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := GatewayClassAcceptedStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.InvalidParameters.Error(), unmarshaled.InvalidParameters.Error())

	require.Equal(t, status.Waiting.Error(), unmarshaled.Waiting.Error())
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

func TestGatewayReadyStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := GatewayReadyStatus{
		ListenersNotValid:  errors.New("ListenersNotValid"),
		ListenersNotReady:  errors.New("ListenersNotReady"),
		AddressNotAssigned: errors.New("AddressNotAssigned"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := GatewayReadyStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.ListenersNotValid.Error(), unmarshaled.ListenersNotValid.Error())

	require.Equal(t, status.ListenersNotReady.Error(), unmarshaled.ListenersNotReady.Error())

	require.Equal(t, status.AddressNotAssigned.Error(), unmarshaled.AddressNotAssigned.Error())
}

func TestGatewayScheduledStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := GatewayScheduledStatus{
		NotReconciled: errors.New("NotReconciled"),
		PodFailed:     errors.New("PodFailed"),
		Unknown:       errors.New("Unknown"),
		NoResources:   errors.New("NoResources"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := GatewayScheduledStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.NotReconciled.Error(), unmarshaled.NotReconciled.Error())

	require.Equal(t, status.PodFailed.Error(), unmarshaled.PodFailed.Error())

	require.Equal(t, status.Unknown.Error(), unmarshaled.Unknown.Error())

	require.Equal(t, status.NoResources.Error(), unmarshaled.NoResources.Error())
}

func TestGatewayInSyncStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := GatewayInSyncStatus{
		SyncError: errors.New("SyncError"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := GatewayInSyncStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.SyncError.Error(), unmarshaled.SyncError.Error())
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

	status = RouteResolvedRefsStatus{RefNotPermitted: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonRefNotPermitted, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = RouteResolvedRefsStatus{InvalidKind: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonInvalidKind, status.Condition(0).Reason)
	require.True(t, status.HasError())

	status = RouteResolvedRefsStatus{BackendNotFound: expected}
	require.Equal(t, "expected", status.Condition(0).Message)
	require.Equal(t, RouteConditionReasonBackendNotFound, status.Condition(0).Reason)
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

func TestRouteAcceptedStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := RouteAcceptedStatus{
		InvalidRouteKind:         errors.New("InvalidRouteKind"),
		ListenerNamespacePolicy:  errors.New("ListenerNamespacePolicy"),
		ListenerHostnameMismatch: errors.New("ListenerHostnameMismatch"),
		BindError:                errors.New("BindError"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := RouteAcceptedStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.InvalidRouteKind.Error(), unmarshaled.InvalidRouteKind.Error())

	require.Equal(t, status.ListenerNamespacePolicy.Error(), unmarshaled.ListenerNamespacePolicy.Error())

	require.Equal(t, status.ListenerHostnameMismatch.Error(), unmarshaled.ListenerHostnameMismatch.Error())

	require.Equal(t, status.BindError.Error(), unmarshaled.BindError.Error())
}

func TestRouteResolvedRefsStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := RouteResolvedRefsStatus{
		Errors:                errors.New("Errors"),
		ServiceNotFound:       errors.New("ServiceNotFound"),
		ConsulServiceNotFound: errors.New("ConsulServiceNotFound"),
		RefNotPermitted:       errors.New("RefNotPermitted"),
		InvalidKind:           errors.New("InvalidKind"),
		BackendNotFound:       errors.New("BackendNotFound"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := RouteResolvedRefsStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.Errors.Error(), unmarshaled.Errors.Error())

	require.Equal(t, status.ServiceNotFound.Error(), unmarshaled.ServiceNotFound.Error())

	require.Equal(t, status.ConsulServiceNotFound.Error(), unmarshaled.ConsulServiceNotFound.Error())

	require.Equal(t, status.RefNotPermitted.Error(), unmarshaled.RefNotPermitted.Error())

	require.Equal(t, status.InvalidKind.Error(), unmarshaled.InvalidKind.Error())

	require.Equal(t, status.BackendNotFound.Error(), unmarshaled.BackendNotFound.Error())
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

func TestListenerConflictedStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := ListenerConflictedStatus{
		HostnameConflict: errors.New("HostnameConflict"),
		ProtocolConflict: errors.New("ProtocolConflict"),
		RouteConflict:    errors.New("RouteConflict"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := ListenerConflictedStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.HostnameConflict.Error(), unmarshaled.HostnameConflict.Error())

	require.Equal(t, status.ProtocolConflict.Error(), unmarshaled.ProtocolConflict.Error())

	require.Equal(t, status.RouteConflict.Error(), unmarshaled.RouteConflict.Error())
}

func TestListenerDetachedStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := ListenerDetachedStatus{
		PortUnavailable:      errors.New("PortUnavailable"),
		UnsupportedExtension: errors.New("UnsupportedExtension"),
		UnsupportedProtocol:  errors.New("UnsupportedProtocol"),
		UnsupportedAddress:   errors.New("UnsupportedAddress"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := ListenerDetachedStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.PortUnavailable.Error(), unmarshaled.PortUnavailable.Error())

	require.Equal(t, status.UnsupportedExtension.Error(), unmarshaled.UnsupportedExtension.Error())

	require.Equal(t, status.UnsupportedProtocol.Error(), unmarshaled.UnsupportedProtocol.Error())

	require.Equal(t, status.UnsupportedAddress.Error(), unmarshaled.UnsupportedAddress.Error())
}

func TestListenerReadyStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := ListenerReadyStatus{
		Invalid: errors.New("Invalid"),
		Pending: errors.New("Pending"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := ListenerReadyStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.Invalid.Error(), unmarshaled.Invalid.Error())

	require.Equal(t, status.Pending.Error(), unmarshaled.Pending.Error())
}

func TestListenerResolvedRefsStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := ListenerResolvedRefsStatus{
		InvalidCertificateRef: errors.New("InvalidCertificateRef"),
		InvalidRouteKinds:     errors.New("InvalidRouteKinds"),
		RefNotPermitted:       errors.New("RefNotPermitted"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)
	unmarshaled := ListenerResolvedRefsStatus{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, status.InvalidCertificateRef.Error(), unmarshaled.InvalidCertificateRef.Error())

	require.Equal(t, status.InvalidRouteKinds.Error(), unmarshaled.InvalidRouteKinds.Error())

	require.Equal(t, status.RefNotPermitted.Error(), unmarshaled.RefNotPermitted.Error())
}
