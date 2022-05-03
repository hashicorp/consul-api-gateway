# Why is my gateway not routing traffic?

When a gateway is not able to route traffic, it will set `Ready {status: “False”}` in the `conditions` array on the gateway `status` field. If a gateway is not ready, it could be caused by either an issue creating the gateway instance itself, or a listener configuration issue.

> TODO: kubectl get gateway $GATEWAY_NAME | jq .status

### Gateway creation issues

For diagnosing gateway issues, first check if `Scheduled {status: false}`is set in the `conditions` array on the gateway `status` field is set. If the gateway has not been scheduled, it may be because of a lack of available underlying hardware resources (indicated by `{reason: NoResources}` or (indicated by `{reason: NotReconciled}`. If either of these reasons are present, check the gateway controller logs for more detail on why the gateway was not able to be created.
- `NoResources` when the gateway is not scheduled because sufficient underlying hardware resources are not available
- `NotReconciled` when no controller has reconciled the gateway
- `PodFailed` when the underlying Kubernetes pod for the gateway has failed, more specific than `NoResources`
- `Unknown` when the underlying pod has an unhandled pod status (FIXME: should this set `NoResources` instead?)

Next, confirm that the gateway is publicly routable by checking if the `Ready {status: False}` condition displays `{reason: AddressNotAssigned}`, that the `addresses` array on the gateway `status` field is populated with at least one valid address routable from “outside the gateway”, and that any requested bindings in the `addresses` array on the gateway `spec` field are present.

If the gateway displays a `Scheduled {status: true}` GatewayConditionType and has at least one entry in the `addresses` status, it should be able to be created successfully and publicly reachable, even if it isn’t yet directing traffic to services successfully. There may be a brief period where a gateway is marked as `Scheduled {status: true} but is still in the process of being created and not ready yet.

### Gateway listener issues

After a gateway instance has been created successfully, a common cause for routing issues is misconfiguration of a listener on the gateway, which is indicated by `Ready {status: false, reason: ListenersNotValid}` in the `conditions` array on the gateway `status` field. Another possible setting is `{reason: ListenersNotReady}`, which is generally a temporary state when the configuration has been accepted as valid but the underlying instance has not yet been configured (FIXME: which gateway condition do we set when a listener sets `Detached {status: true}`?).

When a listener configuration is invalid, it’s necessary to look at the `listeners` array on the gateway `status` field for more details. This field contains a status for each listener on the gateway, identified by `name`.

There are several different types of issues that can cause a listener status to set `Ready {status: false, reason: Invalid}`. (Another possibly state is `reason: Pending`, which is a temporary state during which the configuration is either being evaluated or the underlying instance is being configured.)

One of these issues is when a listener that supports TLS configuration attempts to reference an invalid or unpermitted secret in the `certificateRefs` array on the `tls` field. In this case, the status for that listener will display ResolvedRefs {status: false, reason: InvalidCertificateRef}`.

- FIXME(upstream): InvalidRouteKinds should instead be RouteConditionReason InvalidRouteKind (we already implement this) and set Accepted {status: false}` instead of sending the listener into an invalid state.
- FIXME(upstream): add `{reason: InvalidTLSConfig}` when the `protocol` field specifies `HTTPS` or `TLS and the `tls` field is missing, or if the `protocol` field is `HTTP`, TCP` or `UDP` and the `tls` field is set

Another listener configuration issue can arise from when the controller is not able to configure the listener on the underlying gateway infrastructure, even though the listener is syntactically and semantically valid. The controller may be unable to attach the listener if it specifies an unsupported requirement, or prerequisite resources are not available. Possible causes will set the following reasons alongside a `Detached {status: false}` condition:
- `PortUnavailble` when an unavailable port is requested. (TODO: does our implementation support this?) Multiple listeners may share the same port as long as their protocols are compatible.
- `UnsupportedExtension` when the controller detects that an implementation-specific Listener extension is being requested, but is not able to support the extension. (FIXME: remove this? I don’t even see a way to specify a Listener extension)
- `UnsupportedProtocol` when an unsupported protocol type is specified in the `protocol` field of the listener
- `UnsupportedAddress`
    - FIXME(upstream): this seems to be referring to what is now the `addresses` field on Gateway, and should be a GatewayConditionReason, not ListenerConditionReason - should this instead be something like an `InvalidHostname` reason?

Finally, another common cause of listener configuration issues is when multiple configuration conflict, in which case the `Conflicted {status: true}` condition will be set. (If no conflicts are found a `Conflicted{status: false, reason: NoConflicts}` condition will be set, which indicates that listener configurations are not in conflict.) The `Conflicted {status: true}` condition may be set with the following reasons:
- `PortConflicted` when multiple listeners with the same `port` value are specified on a gateway but are not compatible
- `HostnameConflict` when multiple listeners on the same port specify the same entry in the hostname field
- `ProtocolConflict` when multiple Listeners are specified with the same listener port number, but specify incompatible protocol types
- `RouteConflict`
    - FIXME(upstream): this looks to reference v1alpha1 spec configuration where a listener selects routes - in v1alpha2 an unsupported route type should just not be accepted by the listener

### Route configuration issues

If the gateway has been created and the listeners configured successfully, check that all route configurations are being accepted by the gateway.

First, check that the `attachedRoutes` field on each status in the `listeners` array on the gateway `status` field is an accurate count of all expected routes referencing that gateway and listener. Next, if any routes are missing, search for any route _without_ `Accepted {status: true}` in the `conditions` array on an item in the `parents` array of the `status` field on the route.

> TODO: kubectl get routes | jq .status.parents.forEach(!conditions.contains Accepted {status: true})

There are a number of cases where the `Accepted` condition may not be set due to lack of visibility from the gateway controller to the route, which includes when:
- The Route refers to a non-existent parent.
- The Route is of a type that the controller does not support.
- The Route is in a namespace the the controller does not have access to.

If the controller does have visibility to the route, the route may either be rejected with an `Accepted{ status: false }` condition or partially accepted (in the case where at least one of the route’s rules is implemented by the gateway), in which case the route will set `Accepted{ status: true }` but also set an additional condition indicating the invalid state, such as `ResolvedRefs{ status: false, reason: RefNotPermitted }` if any rule on the route references an invalid or unpermitted backend. If a `RefsNotPermitted` reason is present on a route, check the gateway controller logs for more detail on which specific rule failed to resolve.

FIXME(upstream): add RouteConditionReason InvalidHostname (or ListenerHostnameMismatch as we set) for `Accepted {status: false}`, see `HTTPRoute` and `TLSRoute` note on the `hostname` field at https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io%2fv1alpha2.Listener
