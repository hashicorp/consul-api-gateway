package reconciler

// GENERATED from statuses.yaml, DO NOT EDIT DIRECTLY

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayClassAcceptedStatus - This condition indicates whether the
// GatewayClass has been accepted by the controller requested in the
// spec.controller field.
//
// This condition defaults to Unknown, and MUST be set by a controller when it
// sees a GatewayClass using its controller string. The status of this condition
// MUST be set to True if the controller will support provisioning Gateways
// using this class. Otherwise, this status MUST be set to False. If the status
// is set to False, the controller SHOULD set a Message and Reason as an
// explanation.
//
// [spec]
type GatewayClassAcceptedStatus struct {
	// This reason is used with the “Accepted” condition when the GatewayClass
	// was not accepted because the parametersRef field was invalid, with more
	// detail in the message.
	//
	// [spec]
	InvalidParameters error
	// This reason is used with the “Accepted” condition when the requested
	// controller has not yet made a decision about whether to admit the
	// GatewayClass. It is the default Reason on a new GatewayClass.
	//
	// [spec]
	Waiting error
}

const (
	// GatewayClassConditionAccepted - This condition indicates whether the
	// GatewayClass has been accepted by the controller requested in the
	// spec.controller field.
	//
	// This condition defaults to Unknown, and MUST be set by a controller when it
	// sees a GatewayClass using its controller string. The status of this condition
	// MUST be set to True if the controller will support provisioning Gateways
	// using this class. Otherwise, this status MUST be set to False. If the status
	// is set to False, the controller SHOULD set a Message and Reason as an
	// explanation.
	//
	// [spec]
	GatewayClassConditionAccepted = "Accepted"
	// GatewayClassConditionReasonAccepted - This reason is used with the
	// “Accepted” condition when the condition is true.
	//
	// [spec]
	GatewayClassConditionReasonAccepted = "Accepted"
	// GatewayClassConditionReasonInvalidParameters - This reason is used with the
	// “Accepted” condition when the GatewayClass was not accepted because the
	// parametersRef field was invalid, with more detail in the message.
	//
	// [spec]
	GatewayClassConditionReasonInvalidParameters = "InvalidParameters"
	// GatewayClassConditionReasonWaiting - This reason is used with the
	// “Accepted” condition when the requested controller has not yet made a
	// decision about whether to admit the GatewayClass. It is the default Reason on
	// a new GatewayClass.
	//
	// [spec]
	GatewayClassConditionReasonWaiting = "Waiting"
)

// Condition returns the status condition of the GatewayClassAcceptedStatus
// based off of the underlying errors that are set.
func (s GatewayClassAcceptedStatus) Condition(generation int64) meta.Condition {
	if s.InvalidParameters != nil {
		return meta.Condition{
			Type:               GatewayClassConditionAccepted,
			Status:             meta.ConditionFalse,
			Reason:             GatewayClassConditionReasonInvalidParameters,
			Message:            s.InvalidParameters.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.Waiting != nil {
		return meta.Condition{
			Type:               GatewayClassConditionAccepted,
			Status:             meta.ConditionFalse,
			Reason:             GatewayClassConditionReasonWaiting,
			Message:            s.Waiting.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               GatewayClassConditionAccepted,
		Status:             meta.ConditionTrue,
		Reason:             GatewayClassConditionReasonAccepted,
		Message:            "Accepted",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the GatewayClassAcceptedStatus errors are
// set.
func (s GatewayClassAcceptedStatus) HasError() bool {
	return s.InvalidParameters != nil || s.Waiting != nil
}

// GatewayClassStatus - Defines the observed state of a GatewayClass.
type GatewayClassStatus struct {
	// This condition indicates whether the GatewayClass has been accepted by the
	// controller requested in the spec.controller field.
	//
	// This condition defaults to Unknown, and MUST be set by a controller when it
	// sees a GatewayClass using its controller string. The status of this condition
	// MUST be set to True if the controller will support provisioning Gateways
	// using this class. Otherwise, this status MUST be set to False. If the status
	// is set to False, the controller SHOULD set a Message and Reason as an
	// explanation.
	Accepted GatewayClassAcceptedStatus
}

// Conditions returns the aggregated status conditions of the
// GatewayClassStatus.
func (s GatewayClassStatus) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		s.Accepted.Condition(generation),
	}
}

// GatewayReadyStatus - This condition is true when the Gateway is expected to
// be able to serve traffic. Note that this does not indicate that the Gateway
// configuration is current or even complete (e.g. the controller may still not
// have reconciled the latest version, or some parts of the configuration could
// be missing).
//
// If both the “ListenersNotValid” and “ListenersNotReady” reasons are
// true, the Gateway controller should prefer the “ListenersNotValid”
// reason.
//
// [spec]
type GatewayReadyStatus struct {
	// This reason is used with the “Ready” condition when one or more Listeners
	// have an invalid or unsupported configuration and cannot be configured on the
	// Gateway.
	//
	// [spec]
	ListenersNotValid error
	// This reason is used with the “Ready” condition when one or more Listeners
	// are not ready to serve traffic.
	//
	// [spec]
	ListenersNotReady error
	// This reason is used with the “Ready” condition when none of the requested
	// addresses have been assigned to the Gateway. This reason can be used to
	// express a range of circumstances, including (but not limited to) IPAM address
	// exhaustion, invalid or unsupported address requests, or a named address not
	// being found.
	//
	// [spec]
	AddressNotAssigned error
}

const (
	// GatewayConditionReady - This condition is true when the Gateway is expected
	// to be able to serve traffic. Note that this does not indicate that the
	// Gateway configuration is current or even complete (e.g. the controller may
	// still not have reconciled the latest version, or some parts of the
	// configuration could be missing).
	//
	// If both the “ListenersNotValid” and “ListenersNotReady” reasons are
	// true, the Gateway controller should prefer the “ListenersNotValid”
	// reason.
	//
	// [spec]
	GatewayConditionReady = "Ready"
	// GatewayConditionReasonReady - This reason is used with the “Ready”
	// condition when the condition is true.
	//
	// [spec]
	GatewayConditionReasonReady = "Ready"
	// GatewayConditionReasonListenersNotValid - This reason is used with the
	// “Ready” condition when one or more Listeners have an invalid or
	// unsupported configuration and cannot be configured on the Gateway.
	//
	// [spec]
	GatewayConditionReasonListenersNotValid = "ListenersNotValid"
	// GatewayConditionReasonListenersNotReady - This reason is used with the
	// “Ready” condition when one or more Listeners are not ready to serve
	// traffic.
	//
	// [spec]
	GatewayConditionReasonListenersNotReady = "ListenersNotReady"
	// GatewayConditionReasonAddressNotAssigned - This reason is used with the
	// “Ready” condition when none of the requested addresses have been assigned
	// to the Gateway. This reason can be used to express a range of circumstances,
	// including (but not limited to) IPAM address exhaustion, invalid or
	// unsupported address requests, or a named address not being found.
	//
	// [spec]
	GatewayConditionReasonAddressNotAssigned = "AddressNotAssigned"
)

// Condition returns the status condition of the GatewayReadyStatus based off of
// the underlying errors that are set.
func (s GatewayReadyStatus) Condition(generation int64) meta.Condition {
	if s.ListenersNotValid != nil {
		return meta.Condition{
			Type:               GatewayConditionReady,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonListenersNotValid,
			Message:            s.ListenersNotValid.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ListenersNotReady != nil {
		return meta.Condition{
			Type:               GatewayConditionReady,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonListenersNotReady,
			Message:            s.ListenersNotReady.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.AddressNotAssigned != nil {
		return meta.Condition{
			Type:               GatewayConditionReady,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonAddressNotAssigned,
			Message:            s.AddressNotAssigned.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               GatewayConditionReady,
		Status:             meta.ConditionTrue,
		Reason:             GatewayConditionReasonReady,
		Message:            "Ready",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the GatewayReadyStatus errors are set.
func (s GatewayReadyStatus) HasError() bool {
	return s.ListenersNotValid != nil || s.ListenersNotReady != nil || s.AddressNotAssigned != nil
}

// GatewayScheduledStatus - This condition is true when the controller managing
// the Gateway has scheduled the Gateway to the underlying network
// infrastructure.
//
// [spec]
type GatewayScheduledStatus struct {
	// This reason is used with the “Scheduled” condition when no controller has
	// reconciled the Gateway.
	//
	// [spec]
	NotReconciled error
	// This reason is used when the underlying pod on the gateway has failed.
	//
	// [custom]
	PodFailed error
	// This reason is used when the underlying pod has an unhandled pod status.
	//
	// [custom]
	Unknown error
	// This reason is used with the “Scheduled” condition when the Gateway is
	// not scheduled because insufficient infrastructure resources are available.
	//
	// [spec]
	NoResources error
}

const (
	// GatewayConditionScheduled - This condition is true when the controller
	// managing the Gateway has scheduled the Gateway to the underlying network
	// infrastructure.
	//
	// [spec]
	GatewayConditionScheduled = "Scheduled"
	// GatewayConditionReasonScheduled - This reason is used with the
	// “Scheduled” condition when the condition is true.
	//
	// [spec]
	GatewayConditionReasonScheduled = "Scheduled"
	// GatewayConditionReasonNotReconciled - This reason is used with the
	// “Scheduled” condition when no controller has reconciled the Gateway.
	//
	// [spec]
	GatewayConditionReasonNotReconciled = "NotReconciled"
	// GatewayConditionReasonPodFailed - This reason is used when the underlying pod
	// on the gateway has failed.
	//
	// [custom]
	GatewayConditionReasonPodFailed = "PodFailed"
	// GatewayConditionReasonUnknown - This reason is used when the underlying pod
	// has an unhandled pod status.
	//
	// [custom]
	GatewayConditionReasonUnknown = "Unknown"
	// GatewayConditionReasonNoResources - This reason is used with the
	// “Scheduled” condition when the Gateway is not scheduled because
	// insufficient infrastructure resources are available.
	//
	// [spec]
	GatewayConditionReasonNoResources = "NoResources"
)

// Condition returns the status condition of the GatewayScheduledStatus based
// off of the underlying errors that are set.
func (s GatewayScheduledStatus) Condition(generation int64) meta.Condition {
	if s.NotReconciled != nil {
		return meta.Condition{
			Type:               GatewayConditionScheduled,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonNotReconciled,
			Message:            s.NotReconciled.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.PodFailed != nil {
		return meta.Condition{
			Type:               GatewayConditionScheduled,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonPodFailed,
			Message:            s.PodFailed.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.Unknown != nil {
		return meta.Condition{
			Type:               GatewayConditionScheduled,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonUnknown,
			Message:            s.Unknown.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.NoResources != nil {
		return meta.Condition{
			Type:               GatewayConditionScheduled,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonNoResources,
			Message:            s.NoResources.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               GatewayConditionScheduled,
		Status:             meta.ConditionTrue,
		Reason:             GatewayConditionReasonScheduled,
		Message:            "Scheduled",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the GatewayScheduledStatus errors are set.
func (s GatewayScheduledStatus) HasError() bool {
	return s.NotReconciled != nil || s.PodFailed != nil || s.Unknown != nil || s.NoResources != nil
}

// GatewayInSyncStatus - This condition is true when the Gateway has
// successfully synced externally.
//
// [spec]
type GatewayInSyncStatus struct {
	// This reason is used with the "InSync" condition when there has been an error
	// encountered synchronizing the Gateway.
	//
	// [spec]
	SyncError error
}

const (
	// GatewayConditionInSync - This condition is true when the Gateway has
	// successfully synced externally.
	//
	// [spec]
	GatewayConditionInSync = "InSync"
	// GatewayConditionReasonInSync - This reason is used with the “InSync”
	// condition when the condition is true.
	//
	// [spec]
	GatewayConditionReasonInSync = "InSync"
	// GatewayConditionReasonSyncError - This reason is used with the "InSync"
	// condition when there has been an error encountered synchronizing the Gateway.
	//
	// [spec]
	GatewayConditionReasonSyncError = "SyncError"
)

// Condition returns the status condition of the GatewayInSyncStatus based off
// of the underlying errors that are set.
func (s GatewayInSyncStatus) Condition(generation int64) meta.Condition {
	if s.SyncError != nil {
		return meta.Condition{
			Type:               GatewayConditionInSync,
			Status:             meta.ConditionFalse,
			Reason:             GatewayConditionReasonSyncError,
			Message:            s.SyncError.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               GatewayConditionInSync,
		Status:             meta.ConditionTrue,
		Reason:             GatewayConditionReasonInSync,
		Message:            "InSync",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the GatewayInSyncStatus errors are set.
func (s GatewayInSyncStatus) HasError() bool {
	return s.SyncError != nil
}

// GatewayStatus - Defines the observed state of a Gateway.
type GatewayStatus struct {
	// This condition is true when the Gateway is expected to be able to serve
	// traffic. Note that this does not indicate that the Gateway configuration is
	// current or even complete (e.g. the controller may still not have reconciled
	// the latest version, or some parts of the configuration could be missing).
	//
	// If both the “ListenersNotValid” and “ListenersNotReady” reasons are
	// true, the Gateway controller should prefer the “ListenersNotValid”
	// reason.
	Ready GatewayReadyStatus
	// This condition is true when the controller managing the Gateway has scheduled
	// the Gateway to the underlying network infrastructure.
	Scheduled GatewayScheduledStatus
	// This condition is true when the Gateway has successfully synced externally.
	InSync GatewayInSyncStatus
}

// Conditions returns the aggregated status conditions of the GatewayStatus.
func (s GatewayStatus) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		s.Ready.Condition(generation),
		s.Scheduled.Condition(generation),
		s.InSync.Condition(generation),
	}
}

// RouteAcceptedStatus - This condition indicates whether the route has been
// accepted or rejected by a Gateway, and why.
//
// [spec]
type RouteAcceptedStatus struct {
	// This reason is used when a Route is rejected from binding to a Listener
	// because it does not match a Listener's allowed route kinds.
	//
	// [custom]
	InvalidRouteKind error
	// This reason is used when a Route is rejected from binding to a Gateway
	// because it does not match a Listener's namespace attachment policy.
	//
	// [custom]
	ListenerNamespacePolicy error
	// This reason is used when a Route is rejected from binding to a Listener
	// becasue it does not match a Listener's hostname.
	//
	// [custom]
	ListenerHostnameMismatch error
	// This reason is used when there is a generic binding error for a Route.
	//
	// [custom]
	BindError error
}

const (
	// RouteConditionAccepted - This condition indicates whether the route has been
	// accepted or rejected by a Gateway, and why.
	//
	// [spec]
	RouteConditionAccepted = "Accepted"
	// RouteConditionReasonAccepted - This reason is used with the "Accepted"
	// condition when the condition is True.
	//
	// [custom]
	RouteConditionReasonAccepted = "Accepted"
	// RouteConditionReasonInvalidRouteKind - This reason is used when a Route is
	// rejected from binding to a Listener because it does not match a Listener's
	// allowed route kinds.
	//
	// [custom]
	RouteConditionReasonInvalidRouteKind = "InvalidRouteKind"
	// RouteConditionReasonListenerNamespacePolicy - This reason is used when a
	// Route is rejected from binding to a Gateway because it does not match a
	// Listener's namespace attachment policy.
	//
	// [custom]
	RouteConditionReasonListenerNamespacePolicy = "ListenerNamespacePolicy"
	// RouteConditionReasonListenerHostnameMismatch - This reason is used when a
	// Route is rejected from binding to a Listener becasue it does not match a
	// Listener's hostname.
	//
	// [custom]
	RouteConditionReasonListenerHostnameMismatch = "ListenerHostnameMismatch"
	// RouteConditionReasonBindError - This reason is used when there is a generic
	// binding error for a Route.
	//
	// [custom]
	RouteConditionReasonBindError = "BindError"
)

// Condition returns the status condition of the RouteAcceptedStatus based off
// of the underlying errors that are set.
func (s RouteAcceptedStatus) Condition(generation int64) meta.Condition {
	if s.InvalidRouteKind != nil {
		return meta.Condition{
			Type:               RouteConditionAccepted,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonInvalidRouteKind,
			Message:            s.InvalidRouteKind.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ListenerNamespacePolicy != nil {
		return meta.Condition{
			Type:               RouteConditionAccepted,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonListenerNamespacePolicy,
			Message:            s.ListenerNamespacePolicy.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ListenerHostnameMismatch != nil {
		return meta.Condition{
			Type:               RouteConditionAccepted,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonListenerHostnameMismatch,
			Message:            s.ListenerHostnameMismatch.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.BindError != nil {
		return meta.Condition{
			Type:               RouteConditionAccepted,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonBindError,
			Message:            s.BindError.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               RouteConditionAccepted,
		Status:             meta.ConditionTrue,
		Reason:             RouteConditionReasonAccepted,
		Message:            "Route accepted.",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// RouteResolvedRefsStatus - This condition indicates whether the controller was
// able to resolve all the object references for the Route.
//
// [spec]
type RouteResolvedRefsStatus struct {
	// This reason is used when multiple resolution errors occur of varying types.
	// See the condition message for more details.
	//
	// [custom]
	Errors error
	// This reason is used when a Route references Kubernetes services that the
	// controller cannot resolve.
	//
	// [custom]
	ServiceNotFound error
	// This reason is used when a Route references services that the controller
	// cannot resolve as services inside of Consul.
	//
	// [custom]
	ConsulServiceNotFound error
}

const (
	// RouteConditionResolvedRefs - This condition indicates whether the controller
	// was able to resolve all the object references for the Route.
	//
	// [spec]
	RouteConditionResolvedRefs = "ResolvedRefs"
	// RouteConditionReasonResolvedRefs - This reason is used with the
	// "ResolvedRefs" condition when the condition is True.
	//
	// [custom]
	RouteConditionReasonResolvedRefs = "ResolvedRefs"
	// RouteConditionReasonErrors - This reason is used when multiple resolution
	// errors occur of varying types. See the condition message for more details.
	//
	// [custom]
	RouteConditionReasonErrors = "Errors"
	// RouteConditionReasonServiceNotFound - This reason is used when a Route
	// references Kubernetes services that the controller cannot resolve.
	//
	// [custom]
	RouteConditionReasonServiceNotFound = "ServiceNotFound"
	// RouteConditionReasonConsulServiceNotFound - This reason is used when a Route
	// references services that the controller cannot resolve as services inside of
	// Consul.
	//
	// [custom]
	RouteConditionReasonConsulServiceNotFound = "ConsulServiceNotFound"
)

// Condition returns the status condition of the RouteResolvedRefsStatus based
// off of the underlying errors that are set.
func (s RouteResolvedRefsStatus) Condition(generation int64) meta.Condition {
	if s.Errors != nil {
		return meta.Condition{
			Type:               RouteConditionResolvedRefs,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonErrors,
			Message:            s.Errors.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ServiceNotFound != nil {
		return meta.Condition{
			Type:               RouteConditionResolvedRefs,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonServiceNotFound,
			Message:            s.ServiceNotFound.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ConsulServiceNotFound != nil {
		return meta.Condition{
			Type:               RouteConditionResolvedRefs,
			Status:             meta.ConditionFalse,
			Reason:             RouteConditionReasonConsulServiceNotFound,
			Message:            s.ConsulServiceNotFound.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               RouteConditionResolvedRefs,
		Status:             meta.ConditionTrue,
		Reason:             RouteConditionReasonResolvedRefs,
		Message:            "ResolvedRefs",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the RouteResolvedRefsStatus errors are set.
func (s RouteResolvedRefsStatus) HasError() bool {
	return s.Errors != nil || s.ServiceNotFound != nil || s.ConsulServiceNotFound != nil
}

// RouteStatus - The status associated with a Route with respect to a given
// parent.
type RouteStatus struct {
	// This condition indicates whether the route has been accepted or rejected by a
	// Gateway, and why.
	Accepted RouteAcceptedStatus
	// This condition indicates whether the controller was able to resolve all the
	// object references for the Route.
	ResolvedRefs RouteResolvedRefsStatus
}

// Conditions returns the aggregated status conditions of the RouteStatus.
func (s RouteStatus) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		s.Accepted.Condition(generation),
		s.ResolvedRefs.Condition(generation),
	}
}

// ListenerConflictedStatus - This condition indicates that the controller was
// unable to resolve conflicting specification requirements for this Listener.
// If a Listener is conflicted, its network port should not be configured on any
// network elements.
//
// [spec]
type ListenerConflictedStatus struct {
	// This reason is used with the “Conflicted” condition when the Listener
	// conflicts with hostnames in other Listeners. For example, this reason would
	// be used when multiple Listeners on the same port use example.com in the
	// hostname field.
	//
	// [spec]
	HostnameConflict error
	// This reason is used with the “Conflicted” condition when multiple
	// Listeners are specified with the same Listener port number, but have
	// conflicting protocol specifications.
	//
	// [spec]
	ProtocolConflict error
	// This reason is used with the “Conflicted” condition when the route
	// resources selected for this Listener conflict with other specified properties
	// of the Listener (e.g. Protocol). For example, a Listener that specifies
	// “UDP” as the protocol but a route selector that resolves “TCPRoute”
	// objects.
	//
	// [spec]
	RouteConflict error
}

const (
	// ListenerConditionConflicted - This condition indicates that the controller
	// was unable to resolve conflicting specification requirements for this
	// Listener. If a Listener is conflicted, its network port should not be
	// configured on any network elements.
	//
	// [spec]
	ListenerConditionConflicted = "Conflicted"
	// ListenerConditionReasonNoConflicts - This reason is used with the
	// “Conflicted” condition when the condition is False.
	//
	// [spec]
	ListenerConditionReasonNoConflicts = "NoConflicts"
	// ListenerConditionReasonHostnameConflict - This reason is used with the
	// “Conflicted” condition when the Listener conflicts with hostnames in
	// other Listeners. For example, this reason would be used when multiple
	// Listeners on the same port use example.com in the hostname field.
	//
	// [spec]
	ListenerConditionReasonHostnameConflict = "HostnameConflict"
	// ListenerConditionReasonProtocolConflict - This reason is used with the
	// “Conflicted” condition when multiple Listeners are specified with the
	// same Listener port number, but have conflicting protocol specifications.
	//
	// [spec]
	ListenerConditionReasonProtocolConflict = "ProtocolConflict"
	// ListenerConditionReasonRouteConflict - This reason is used with the
	// “Conflicted” condition when the route resources selected for this
	// Listener conflict with other specified properties of the Listener (e.g.
	// Protocol). For example, a Listener that specifies “UDP” as the protocol
	// but a route selector that resolves “TCPRoute” objects.
	//
	// [spec]
	ListenerConditionReasonRouteConflict = "RouteConflict"
)

// Condition returns the status condition of the ListenerConflictedStatus based
// off of the underlying errors that are set.
func (s ListenerConflictedStatus) Condition(generation int64) meta.Condition {
	if s.HostnameConflict != nil {
		return meta.Condition{
			Type:               ListenerConditionConflicted,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonHostnameConflict,
			Message:            s.HostnameConflict.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.ProtocolConflict != nil {
		return meta.Condition{
			Type:               ListenerConditionConflicted,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonProtocolConflict,
			Message:            s.ProtocolConflict.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.RouteConflict != nil {
		return meta.Condition{
			Type:               ListenerConditionConflicted,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonRouteConflict,
			Message:            s.RouteConflict.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               ListenerConditionConflicted,
		Status:             meta.ConditionFalse,
		Reason:             ListenerConditionReasonNoConflicts,
		Message:            "NoConflicts",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the ListenerConflictedStatus errors are set.
func (s ListenerConflictedStatus) HasError() bool {
	return s.HostnameConflict != nil || s.ProtocolConflict != nil || s.RouteConflict != nil
}

// ListenerDetachedStatus - This condition indicates that, even though the
// listener is syntactically and semantically valid, the controller is not able
// to configure it on the underlying Gateway infrastructure.
//
// A Listener is specified as a logical requirement, but needs to be configured
// on a network endpoint (i.e. address and port) by a controller. The controller
// may be unable to attach the Listener if it specifies an unsupported
// requirement, or prerequisite resources are not available.
//
// [spec]
type ListenerDetachedStatus struct {
	// This reason is used with the “Detached” condition when the Listener
	// requests a port that cannot be used on the Gateway. This reason could be used
	// in a number of instances, including:
	//
	// - The port is already in use.
	//
	// - The port is not supported by the implementation.
	//
	// [spec]
	PortUnavailable error
	// This reason is used with the “Detached” condition when the controller
	// detects that an implementation-specific Listener extension is being
	// requested, but is not able to support the extension.
	//
	// [spec]
	UnsupportedExtension error
	// This reason is used with the “Detached” condition when the Listener could
	// not be attached to be Gateway because its protocol type is not supported.
	//
	// [spec]
	UnsupportedProtocol error
	// This reason is used with the “Detached” condition when the Listener could
	// not be attached to the Gateway because the requested address is not
	// supported. This reason could be used in a number of instances, including:
	//
	// - The address is already in use.
	//
	// - The type of address is not supported by the implementation.
	//
	// [spec]
	UnsupportedAddress error
}

const (
	// ListenerConditionDetached - This condition indicates that, even though the
	// listener is syntactically and semantically valid, the controller is not able
	// to configure it on the underlying Gateway infrastructure.
	//
	// A Listener is specified as a logical requirement, but needs to be configured
	// on a network endpoint (i.e. address and port) by a controller. The controller
	// may be unable to attach the Listener if it specifies an unsupported
	// requirement, or prerequisite resources are not available.
	//
	// [spec]
	ListenerConditionDetached = "Detached"
	// ListenerConditionReasonAttached - This reason is used with the “Detached”
	// condition when the condition is False.
	//
	// [spec]
	ListenerConditionReasonAttached = "Attached"
	// ListenerConditionReasonPortUnavailable - This reason is used with the
	// “Detached” condition when the Listener requests a port that cannot be
	// used on the Gateway. This reason could be used in a number of instances,
	// including:
	//
	// - The port is already in use.
	//
	// - The port is not supported by the implementation.
	//
	// [spec]
	ListenerConditionReasonPortUnavailable = "PortUnavailable"
	// ListenerConditionReasonUnsupportedExtension - This reason is used with the
	// “Detached” condition when the controller detects that an
	// implementation-specific Listener extension is being requested, but is not
	// able to support the extension.
	//
	// [spec]
	ListenerConditionReasonUnsupportedExtension = "UnsupportedExtension"
	// ListenerConditionReasonUnsupportedProtocol - This reason is used with the
	// “Detached” condition when the Listener could not be attached to be
	// Gateway because its protocol type is not supported.
	//
	// [spec]
	ListenerConditionReasonUnsupportedProtocol = "UnsupportedProtocol"
	// ListenerConditionReasonUnsupportedAddress - This reason is used with the
	// “Detached” condition when the Listener could not be attached to the
	// Gateway because the requested address is not supported. This reason could be
	// used in a number of instances, including:
	//
	// - The address is already in use.
	//
	// - The type of address is not supported by the implementation.
	//
	// [spec]
	ListenerConditionReasonUnsupportedAddress = "UnsupportedAddress"
)

// Condition returns the status condition of the ListenerDetachedStatus based
// off of the underlying errors that are set.
func (s ListenerDetachedStatus) Condition(generation int64) meta.Condition {
	if s.PortUnavailable != nil {
		return meta.Condition{
			Type:               ListenerConditionDetached,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonPortUnavailable,
			Message:            s.PortUnavailable.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.UnsupportedExtension != nil {
		return meta.Condition{
			Type:               ListenerConditionDetached,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonUnsupportedExtension,
			Message:            s.UnsupportedExtension.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.UnsupportedProtocol != nil {
		return meta.Condition{
			Type:               ListenerConditionDetached,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonUnsupportedProtocol,
			Message:            s.UnsupportedProtocol.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.UnsupportedAddress != nil {
		return meta.Condition{
			Type:               ListenerConditionDetached,
			Status:             meta.ConditionTrue,
			Reason:             ListenerConditionReasonUnsupportedAddress,
			Message:            s.UnsupportedAddress.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               ListenerConditionDetached,
		Status:             meta.ConditionFalse,
		Reason:             ListenerConditionReasonAttached,
		Message:            "Attached",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the ListenerDetachedStatus errors are set.
func (s ListenerDetachedStatus) HasError() bool {
	return s.PortUnavailable != nil || s.UnsupportedExtension != nil || s.UnsupportedProtocol != nil || s.UnsupportedAddress != nil
}

// ListenerReadyStatus - This condition indicates whether the Listener has been
// configured on the Gateway.
//
// [spec]
type ListenerReadyStatus struct {
	// This reason is used with the “Ready” condition when the Listener is
	// syntactically or semantically invalid.
	//
	// [spec]
	Invalid error
	// This reason is used with the “Ready” condition when the Listener is not
	// yet not online and ready to accept client traffic.
	//
	// [spec]
	Pending error
}

const (
	// ListenerConditionReady - This condition indicates whether the Listener has
	// been configured on the Gateway.
	//
	// [spec]
	ListenerConditionReady = "Ready"
	// ListenerConditionReasonReady - This reason is used with the “Ready”
	// condition when the condition is true.
	//
	// [spec]
	ListenerConditionReasonReady = "Ready"
	// ListenerConditionReasonInvalid - This reason is used with the “Ready”
	// condition when the Listener is syntactically or semantically invalid.
	//
	// [spec]
	ListenerConditionReasonInvalid = "Invalid"
	// ListenerConditionReasonPending - This reason is used with the “Ready”
	// condition when the Listener is not yet not online and ready to accept client
	// traffic.
	//
	// [spec]
	ListenerConditionReasonPending = "Pending"
)

// Condition returns the status condition of the ListenerReadyStatus based off
// of the underlying errors that are set.
func (s ListenerReadyStatus) Condition(generation int64) meta.Condition {
	if s.Invalid != nil {
		return meta.Condition{
			Type:               ListenerConditionReady,
			Status:             meta.ConditionFalse,
			Reason:             ListenerConditionReasonInvalid,
			Message:            s.Invalid.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.Pending != nil {
		return meta.Condition{
			Type:               ListenerConditionReady,
			Status:             meta.ConditionFalse,
			Reason:             ListenerConditionReasonPending,
			Message:            s.Pending.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               ListenerConditionReady,
		Status:             meta.ConditionTrue,
		Reason:             ListenerConditionReasonReady,
		Message:            "Ready",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the ListenerReadyStatus errors are set.
func (s ListenerReadyStatus) HasError() bool {
	return s.Invalid != nil || s.Pending != nil
}

// ListenerResolvedRefsStatus - This condition indicates whether the controller
// was able to resolve all the object references for the Listener.
//
// [spec]
type ListenerResolvedRefsStatus struct {
	// This reason is used with the “ResolvedRefs” condition when the Listener
	// has a TLS configuration with at least one TLS CertificateRef that is invalid
	// or cannot be resolved.
	//
	// [spec]
	InvalidCertificateRef error
	// This reason is used with the “ResolvedRefs” condition when an invalid or
	// unsupported Route kind is specified by the Listener.
	//
	// [spec]
	InvalidRouteKinds error
	// This reason is used with the “ResolvedRefs” condition when one of the
	// Listener’s Routes has a BackendRef to an object in another namespace, where
	// the object in the other namespace does not have a ReferencePolicy explicitly
	// allowing the reference.
	//
	// [spec]
	RefNotPermitted error
}

const (
	// ListenerConditionResolvedRefs - This condition indicates whether the
	// controller was able to resolve all the object references for the Listener.
	//
	// [spec]
	ListenerConditionResolvedRefs = "ResolvedRefs"
	// ListenerConditionReasonResolvedRefs - This reason is used with the
	// “ResolvedRefs” condition when the condition is true.
	//
	// [spec]
	ListenerConditionReasonResolvedRefs = "ResolvedRefs"
	// ListenerConditionReasonInvalidCertificateRef - This reason is used with the
	// “ResolvedRefs” condition when the Listener has a TLS configuration with
	// at least one TLS CertificateRef that is invalid or cannot be resolved.
	//
	// [spec]
	ListenerConditionReasonInvalidCertificateRef = "InvalidCertificateRef"
	// ListenerConditionReasonInvalidRouteKinds - This reason is used with the
	// “ResolvedRefs” condition when an invalid or unsupported Route kind is
	// specified by the Listener.
	//
	// [spec]
	ListenerConditionReasonInvalidRouteKinds = "InvalidRouteKinds"
	// ListenerConditionReasonRefNotPermitted - This reason is used with the
	// “ResolvedRefs” condition when one of the Listener’s Routes has a
	// BackendRef to an object in another namespace, where the object in the other
	// namespace does not have a ReferencePolicy explicitly allowing the reference.
	//
	// [spec]
	ListenerConditionReasonRefNotPermitted = "RefNotPermitted"
)

// Condition returns the status condition of the ListenerResolvedRefsStatus
// based off of the underlying errors that are set.
func (s ListenerResolvedRefsStatus) Condition(generation int64) meta.Condition {
	if s.InvalidCertificateRef != nil {
		return meta.Condition{
			Type:               ListenerConditionResolvedRefs,
			Status:             meta.ConditionFalse,
			Reason:             ListenerConditionReasonInvalidCertificateRef,
			Message:            s.InvalidCertificateRef.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.InvalidRouteKinds != nil {
		return meta.Condition{
			Type:               ListenerConditionResolvedRefs,
			Status:             meta.ConditionFalse,
			Reason:             ListenerConditionReasonInvalidRouteKinds,
			Message:            s.InvalidRouteKinds.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	if s.RefNotPermitted != nil {
		return meta.Condition{
			Type:               ListenerConditionResolvedRefs,
			Status:             meta.ConditionFalse,
			Reason:             ListenerConditionReasonRefNotPermitted,
			Message:            s.RefNotPermitted.Error(),
			ObservedGeneration: generation,
			LastTransitionTime: meta.Now(),
		}
	}

	return meta.Condition{
		Type:               ListenerConditionResolvedRefs,
		Status:             meta.ConditionTrue,
		Reason:             ListenerConditionReasonResolvedRefs,
		Message:            "ResolvedRefs",
		ObservedGeneration: generation,
		LastTransitionTime: meta.Now(),
	}
}

// HasError returns whether any of the ListenerResolvedRefsStatus errors are
// set.
func (s ListenerResolvedRefsStatus) HasError() bool {
	return s.InvalidCertificateRef != nil || s.InvalidRouteKinds != nil || s.RefNotPermitted != nil
}

// ListenerStatus - The status associated with a Listener.
type ListenerStatus struct {
	// This condition indicates that the controller was unable to resolve
	// conflicting specification requirements for this Listener. If a Listener is
	// conflicted, its network port should not be configured on any network
	// elements.
	Conflicted ListenerConflictedStatus
	// This condition indicates that, even though the listener is syntactically and
	// semantically valid, the controller is not able to configure it on the
	// underlying Gateway infrastructure.
	//
	// A Listener is specified as a logical requirement, but needs to be configured
	// on a network endpoint (i.e. address and port) by a controller. The controller
	// may be unable to attach the Listener if it specifies an unsupported
	// requirement, or prerequisite resources are not available.
	Detached ListenerDetachedStatus
	// This condition indicates whether the Listener has been configured on the
	// Gateway.
	Ready ListenerReadyStatus
	// This condition indicates whether the controller was able to resolve all the
	// object references for the Listener.
	ResolvedRefs ListenerResolvedRefsStatus
}

// Conditions returns the aggregated status conditions of the ListenerStatus.
func (s ListenerStatus) Conditions(generation int64) []meta.Condition {
	return []meta.Condition{
		s.Conflicted.Condition(generation),
		s.Detached.Condition(generation),
		s.Ready.Condition(generation),
		s.ResolvedRefs.Condition(generation),
	}
}

// Valid returns whether all of the required conditions for the ListenerStatus
// are satisfied.
func (s ListenerStatus) Valid() bool {
	if s.Conflicted.HasError() || s.Detached.HasError() || s.ResolvedRefs.HasError() {
		return false
	}
	return true
}
