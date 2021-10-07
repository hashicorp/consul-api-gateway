package reconciler

// GENERATED, DO NOT EDIT DIRECTLY

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type ListenerConflictedStatus struct {
	HostnameConflict error
	ProtocolConflict error
	RouteConflict    error
}

const (
	ConditionReasonNoConflicts      = "NoConflicts"
	ConditionReasonHostnameConflict = "HostnameConflict"
	ConditionReasonProtocolConflict = "ProtocolConflict"
	ConditionReasonRouteConflict    = "RouteConflict"
)

func (s ListenerConflictedStatus) Condition(generation int64) meta.Condition {
	if s.HostnameConflict != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionConflicted),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonHostnameConflict,
			Message:            s.HostnameConflict.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ProtocolConflict != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionConflicted),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonProtocolConflict,
			Message:            s.ProtocolConflict.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.RouteConflict != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionConflicted),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonRouteConflict,
			Message:            s.RouteConflict.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               string(gw.ListenerConditionConflicted),
		Status:             meta.ConditionFalse,
		Reason:             ConditionReasonNoConflicts,
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

func (s ListenerConflictedStatus) HasError() bool {
	return s.HostnameConflict != nil || s.ProtocolConflict != nil || s.RouteConflict != nil
}

type ListenerDetachedStatus struct {
	PortUnvailable       error
	UnsupportedExtension error
	UnsupportedProtocol  error
	UnsupportedAddress   error
}

const (
	ConditionReasonAttached             = "Attached"
	ConditionReasonPortUnvailable       = "PortUnvailable"
	ConditionReasonUnsupportedExtension = "UnsupportedExtension"
	ConditionReasonUnsupportedProtocol  = "UnsupportedProtocol"
	ConditionReasonUnsupportedAddress   = "UnsupportedAddress"
)

func (s ListenerDetachedStatus) Condition(generation int64) meta.Condition {
	if s.PortUnvailable != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionDetached),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonPortUnvailable,
			Message:            s.PortUnvailable.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.UnsupportedExtension != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionDetached),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonUnsupportedExtension,
			Message:            s.UnsupportedExtension.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.UnsupportedProtocol != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionDetached),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonUnsupportedProtocol,
			Message:            s.UnsupportedProtocol.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.UnsupportedAddress != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionDetached),
			Status:             meta.ConditionTrue,
			Reason:             ConditionReasonUnsupportedAddress,
			Message:            s.UnsupportedAddress.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               string(gw.ListenerConditionDetached),
		Status:             meta.ConditionFalse,
		Reason:             ConditionReasonAttached,
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

func (s ListenerDetachedStatus) HasError() bool {
	return s.PortUnvailable != nil || s.UnsupportedExtension != nil || s.UnsupportedProtocol != nil || s.UnsupportedAddress != nil
}

type ListenerReadyStatus struct {
	Invalid error
	Pending error
}

const (
	ConditionReasonReady   = "Ready"
	ConditionReasonInvalid = "Invalid"
	ConditionReasonPending = "Pending"
)

func (s ListenerReadyStatus) Condition(generation int64) meta.Condition {
	if s.Invalid != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionReady),
			Status:             meta.ConditionFalse,
			Reason:             ConditionReasonInvalid,
			Message:            s.Invalid.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.Pending != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionReady),
			Status:             meta.ConditionFalse,
			Reason:             ConditionReasonPending,
			Message:            s.Pending.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               string(gw.ListenerConditionReady),
		Status:             meta.ConditionTrue,
		Reason:             ConditionReasonReady,
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

func (s ListenerReadyStatus) HasError() bool {
	return s.Invalid != nil || s.Pending != nil
}

type ListenerResolvedRefsStatus struct {
	InvalidCertificateRef error
	InvalidRouteKinds     error
	RefNotPermitted       error
}

const (
	ConditionReasonResolvedRefs          = "ResolvedRefs"
	ConditionReasonInvalidCertificateRef = "InvalidCertificateRef"
	ConditionReasonInvalidRouteKinds     = "InvalidRouteKinds"
	ConditionReasonRefNotPermitted       = "RefNotPermitted"
)

func (s ListenerResolvedRefsStatus) Condition(generation int64) meta.Condition {
	if s.InvalidCertificateRef != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionResolvedRefs),
			Status:             meta.ConditionFalse,
			Reason:             ConditionReasonInvalidCertificateRef,
			Message:            s.InvalidCertificateRef.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.InvalidRouteKinds != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionResolvedRefs),
			Status:             meta.ConditionFalse,
			Reason:             ConditionReasonInvalidRouteKinds,
			Message:            s.InvalidRouteKinds.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.RefNotPermitted != nil {
		return meta.Condition{
			Type:               string(gw.ListenerConditionResolvedRefs),
			Status:             meta.ConditionFalse,
			Reason:             ConditionReasonRefNotPermitted,
			Message:            s.RefNotPermitted.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               string(gw.ListenerConditionResolvedRefs),
		Status:             meta.ConditionTrue,
		Reason:             ConditionReasonResolvedRefs,
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

func (s ListenerResolvedRefsStatus) HasError() bool {
	return s.InvalidCertificateRef != nil || s.InvalidRouteKinds != nil || s.RefNotPermitted != nil
}

type ListenerStatus struct {
	Conflicted   ListenerConflictedStatus
	Detached     ListenerDetachedStatus
	Ready        ListenerReadyStatus
	ResolvedRefs ListenerResolvedRefsStatus
}

func (s ListenerStatus) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		s.Conflicted.Condition(generation),
		s.Detached.Condition(generation),
		s.Ready.Condition(generation),
		s.ResolvedRefs.Condition(generation),
	}
}

func (s ListenerStatus) Valid() bool {
	if s.Conflicted.HasError() || s.Detached.HasError() || s.ResolvedRefs.HasError() {
		return false
	}
	return true
}
