package status

// GENERATED from statuses.yaml, DO NOT EDIT DIRECTLY

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayClassAcceptedStatus(t *testing.T) {
	t.Parallel()

	var status GatewayClassAcceptedStatus

	expected := errors.New("expected")

	status = GatewayClassAcceptedStatus{}
	assert.Equal(t, "Accepted", status.Condition(0).Message)
	assert.Equal(t, GatewayClassConditionReasonAccepted, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = GatewayClassAcceptedStatus{InvalidParameters: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayClassConditionReasonInvalidParameters, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = GatewayClassAcceptedStatus{Waiting: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayClassConditionReasonWaiting, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestGatewayClassStatus(t *testing.T) {
	t.Parallel()

	status := GatewayClassStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = GatewayClassConditionAccepted
	reason = GatewayClassConditionReasonAccepted
	assert.Equal(t, conditionType, conditions[0].Type)
	assert.Equal(t, reason, conditions[0].Reason)
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.InvalidParameters.Error(), unmarshaled.InvalidParameters.Error())
	assert.Equal(t, status.Waiting.Error(), unmarshaled.Waiting.Error())
}

func TestGatewayReadyStatus(t *testing.T) {
	t.Parallel()

	var status GatewayReadyStatus

	expected := errors.New("expected")

	status = GatewayReadyStatus{}
	assert.Equal(t, "Ready", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonReady, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = GatewayReadyStatus{ListenersNotValid: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonListenersNotValid, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = GatewayReadyStatus{ListenersNotReady: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonListenersNotReady, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = GatewayReadyStatus{AddressNotAssigned: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonAddressNotAssigned, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestGatewayScheduledStatus(t *testing.T) {
	t.Parallel()

	var status GatewayScheduledStatus

	expected := errors.New("expected")

	status = GatewayScheduledStatus{}
	assert.Equal(t, "Scheduled", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonScheduled, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = GatewayScheduledStatus{NotReconciled: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonNotReconciled, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = GatewayScheduledStatus{PodFailed: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonPodFailed, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = GatewayScheduledStatus{Unknown: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonUnknown, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = GatewayScheduledStatus{NoResources: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonNoResources, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestGatewayInSyncStatus(t *testing.T) {
	t.Parallel()

	var status GatewayInSyncStatus

	expected := errors.New("expected")

	status = GatewayInSyncStatus{}
	assert.Equal(t, "InSync", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonInSync, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = GatewayInSyncStatus{SyncError: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, GatewayConditionReasonSyncError, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestGatewayStatus(t *testing.T) {
	t.Parallel()

	status := GatewayStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = GatewayConditionReady
	reason = GatewayConditionReasonReady
	assert.Equal(t, conditionType, conditions[0].Type)
	assert.Equal(t, reason, conditions[0].Reason)

	conditionType = GatewayConditionScheduled
	reason = GatewayConditionReasonScheduled
	assert.Equal(t, conditionType, conditions[1].Type)
	assert.Equal(t, reason, conditions[1].Reason)

	conditionType = GatewayConditionInSync
	reason = GatewayConditionReasonInSync
	assert.Equal(t, conditionType, conditions[2].Type)
	assert.Equal(t, reason, conditions[2].Reason)
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.ListenersNotValid.Error(), unmarshaled.ListenersNotValid.Error())
	assert.Equal(t, status.ListenersNotReady.Error(), unmarshaled.ListenersNotReady.Error())
	assert.Equal(t, status.AddressNotAssigned.Error(), unmarshaled.AddressNotAssigned.Error())
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.NotReconciled.Error(), unmarshaled.NotReconciled.Error())
	assert.Equal(t, status.PodFailed.Error(), unmarshaled.PodFailed.Error())
	assert.Equal(t, status.Unknown.Error(), unmarshaled.Unknown.Error())
	assert.Equal(t, status.NoResources.Error(), unmarshaled.NoResources.Error())
}

func TestGatewayInSyncStatusMarshaling(t *testing.T) {
	t.Parallel()

	status := GatewayInSyncStatus{
		SyncError: errors.New("SyncError"),
	}

	data, err := json.Marshal(&status)
	require.NoError(t, err)

	unmarshaled := GatewayInSyncStatus{}
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.SyncError.Error(), unmarshaled.SyncError.Error())
}

func TestRouteAcceptedStatus(t *testing.T) {
	t.Parallel()

	var status RouteAcceptedStatus

	expected := errors.New("expected")

	status = RouteAcceptedStatus{}
	assert.Equal(t, "Route accepted.", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonAccepted, status.Condition(0).Reason)

	status = RouteAcceptedStatus{InvalidRouteKind: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonInvalidRouteKind, status.Condition(0).Reason)

	status = RouteAcceptedStatus{ListenerNamespacePolicy: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonListenerNamespacePolicy, status.Condition(0).Reason)

	status = RouteAcceptedStatus{ListenerHostnameMismatch: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonListenerHostnameMismatch, status.Condition(0).Reason)

	status = RouteAcceptedStatus{BindError: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonBindError, status.Condition(0).Reason)

}

func TestRouteResolvedRefsStatus(t *testing.T) {
	t.Parallel()

	var status RouteResolvedRefsStatus

	expected := errors.New("expected")

	status = RouteResolvedRefsStatus{}
	assert.Equal(t, "ResolvedRefs", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonResolvedRefs, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = RouteResolvedRefsStatus{Errors: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonErrors, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = RouteResolvedRefsStatus{ServiceNotFound: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonServiceNotFound, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = RouteResolvedRefsStatus{ConsulServiceNotFound: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonConsulServiceNotFound, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = RouteResolvedRefsStatus{RefNotPermitted: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonRefNotPermitted, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = RouteResolvedRefsStatus{InvalidKind: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonInvalidKind, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = RouteResolvedRefsStatus{BackendNotFound: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, RouteConditionReasonBackendNotFound, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestRouteStatus(t *testing.T) {
	t.Parallel()

	status := RouteStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = RouteConditionAccepted
	reason = RouteConditionReasonAccepted
	assert.Equal(t, conditionType, conditions[0].Type)
	assert.Equal(t, reason, conditions[0].Reason)

	conditionType = RouteConditionResolvedRefs
	reason = RouteConditionReasonResolvedRefs
	assert.Equal(t, conditionType, conditions[1].Type)
	assert.Equal(t, reason, conditions[1].Reason)
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.InvalidRouteKind.Error(), unmarshaled.InvalidRouteKind.Error())
	assert.Equal(t, status.ListenerNamespacePolicy.Error(), unmarshaled.ListenerNamespacePolicy.Error())
	assert.Equal(t, status.ListenerHostnameMismatch.Error(), unmarshaled.ListenerHostnameMismatch.Error())
	assert.Equal(t, status.BindError.Error(), unmarshaled.BindError.Error())
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.Errors.Error(), unmarshaled.Errors.Error())
	assert.Equal(t, status.ServiceNotFound.Error(), unmarshaled.ServiceNotFound.Error())
	assert.Equal(t, status.ConsulServiceNotFound.Error(), unmarshaled.ConsulServiceNotFound.Error())
	assert.Equal(t, status.RefNotPermitted.Error(), unmarshaled.RefNotPermitted.Error())
	assert.Equal(t, status.InvalidKind.Error(), unmarshaled.InvalidKind.Error())
	assert.Equal(t, status.BackendNotFound.Error(), unmarshaled.BackendNotFound.Error())
}

func TestListenerConflictedStatus(t *testing.T) {
	t.Parallel()

	var status ListenerConflictedStatus

	expected := errors.New("expected")

	status = ListenerConflictedStatus{}
	assert.Equal(t, "NoConflicts", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonNoConflicts, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = ListenerConflictedStatus{HostnameConflict: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonHostnameConflict, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerConflictedStatus{ProtocolConflict: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonProtocolConflict, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerConflictedStatus{RouteConflict: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonRouteConflict, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestListenerDetachedStatus(t *testing.T) {
	t.Parallel()

	var status ListenerDetachedStatus

	expected := errors.New("expected")

	status = ListenerDetachedStatus{}
	assert.Equal(t, "Attached", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonAttached, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = ListenerDetachedStatus{PortUnavailable: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonPortUnavailable, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerDetachedStatus{UnsupportedExtension: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonUnsupportedExtension, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerDetachedStatus{UnsupportedProtocol: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonUnsupportedProtocol, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerDetachedStatus{UnsupportedAddress: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonUnsupportedAddress, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestListenerReadyStatus(t *testing.T) {
	t.Parallel()

	var status ListenerReadyStatus

	expected := errors.New("expected")

	status = ListenerReadyStatus{}
	assert.Equal(t, "Ready", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonReady, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = ListenerReadyStatus{Invalid: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonInvalid, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerReadyStatus{Pending: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonPending, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestListenerResolvedRefsStatus(t *testing.T) {
	t.Parallel()

	var status ListenerResolvedRefsStatus

	expected := errors.New("expected")

	status = ListenerResolvedRefsStatus{}
	assert.Equal(t, "ResolvedRefs", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonResolvedRefs, status.Condition(0).Reason)
	assert.False(t, status.HasError())

	status = ListenerResolvedRefsStatus{InvalidCertificateRef: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonInvalidCertificateRef, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerResolvedRefsStatus{InvalidRouteKinds: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonInvalidRouteKinds, status.Condition(0).Reason)
	assert.True(t, status.HasError())

	status = ListenerResolvedRefsStatus{RefNotPermitted: expected}
	assert.Equal(t, "expected", status.Condition(0).Message)
	assert.Equal(t, ListenerConditionReasonRefNotPermitted, status.Condition(0).Reason)
	assert.True(t, status.HasError())

}

func TestListenerStatus(t *testing.T) {
	t.Parallel()

	status := ListenerStatus{}
	conditions := status.Conditions(0)

	var conditionType string
	var reason string

	conditionType = ListenerConditionConflicted
	reason = ListenerConditionReasonNoConflicts
	assert.Equal(t, conditionType, conditions[0].Type)
	assert.Equal(t, reason, conditions[0].Reason)

	conditionType = ListenerConditionDetached
	reason = ListenerConditionReasonAttached
	assert.Equal(t, conditionType, conditions[1].Type)
	assert.Equal(t, reason, conditions[1].Reason)

	conditionType = ListenerConditionReady
	reason = ListenerConditionReasonReady
	assert.Equal(t, conditionType, conditions[2].Type)
	assert.Equal(t, reason, conditions[2].Reason)

	conditionType = ListenerConditionResolvedRefs
	reason = ListenerConditionReasonResolvedRefs
	assert.Equal(t, conditionType, conditions[3].Type)
	assert.Equal(t, reason, conditions[3].Reason)

	require.True(t, status.Valid())

	validationError := errors.New("error")

	status = ListenerStatus{}
	status.Conflicted.HostnameConflict = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.Conflicted.ProtocolConflict = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.Conflicted.RouteConflict = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.PortUnavailable = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.UnsupportedExtension = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.UnsupportedProtocol = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.Detached.UnsupportedAddress = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.ResolvedRefs.InvalidCertificateRef = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.ResolvedRefs.InvalidRouteKinds = validationError
	assert.False(t, status.Valid())

	status = ListenerStatus{}
	status.ResolvedRefs.RefNotPermitted = validationError
	assert.False(t, status.Valid())

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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.HostnameConflict.Error(), unmarshaled.HostnameConflict.Error())
	assert.Equal(t, status.ProtocolConflict.Error(), unmarshaled.ProtocolConflict.Error())
	assert.Equal(t, status.RouteConflict.Error(), unmarshaled.RouteConflict.Error())
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.PortUnavailable.Error(), unmarshaled.PortUnavailable.Error())
	assert.Equal(t, status.UnsupportedExtension.Error(), unmarshaled.UnsupportedExtension.Error())
	assert.Equal(t, status.UnsupportedProtocol.Error(), unmarshaled.UnsupportedProtocol.Error())
	assert.Equal(t, status.UnsupportedAddress.Error(), unmarshaled.UnsupportedAddress.Error())
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.Invalid.Error(), unmarshaled.Invalid.Error())
	assert.Equal(t, status.Pending.Error(), unmarshaled.Pending.Error())
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
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, status.InvalidCertificateRef.Error(), unmarshaled.InvalidCertificateRef.Error())
	assert.Equal(t, status.InvalidRouteKinds.Error(), unmarshaled.InvalidRouteKinds.Error())
	assert.Equal(t, status.RefNotPermitted.Error(), unmarshaled.RefNotPermitted.Error())
}
