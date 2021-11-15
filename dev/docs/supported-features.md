# Supported Features
Below is a list of the Kubernetes Gateway API features supported in the current release of the
Consul API Gateway.

Consul API Gateway version: **v0.1.0-techpreview**
Suppoorted K8s Gateway API version: **v1alpha2**

Supported features are marked with a grey checkbox

## Supported K8s Gateway API Features

- [x] GatewayClass
  - [x] Spec
    - [x] Controller Matching
    - [x] Parameter specification *introduction of `GatewayClassConfig` CRD and validation that only it is used*
  - [x] Finalizers
  - [x] Status
    - [x] Accepted
      - [x] Accepted
      - [x] InvalidParameters
      - [x] ~~Waiting~~ *no need to set directly, since it's the default, unreconciled status*

- [ ] Gateway
  - [ ] Spec
    - [x] GatewayClass/Controller matching
    - [ ] Listeners
      - [x] Name matching on route insertion
      - [x] Port specification *need to test listener merging behavior*
      - [ ] Protocols
        - [x] HTTP
        - [x] HTTPS
        - [ ] TCP
        - [ ] TLS
        - [x] UDP *not supported*
      - [ ] Hostname matching
        - [x] HTTP
        - [x] HTTPS *no SNI checks right now*
        - [ ] TLS
      - [ ] Allowed routes
        - [x] Namespace matching on route insertion
        - [x] Route kind matching on insertion
        - [ ] Route rule selection for multiple route rule matches
      - [ ] TLS
        - [ ] Modes
          - [x] Terminate
          - [ ] Passthrough *explicitly not supported yet*
        - [x] Certificate References *only single Kubernetes secret certificates supported for now*
        - [x] Options *not used*
    - [x] ~~Addresses~~ *not supported*
  - [x] Deployment *based off of a snapshot of GatewayClass configuration at time of Gateway creation as per spec suggestions* 
  - [ ] Status
    - [ ] Addresses
    - [x] Listeners
      - [x] Conflicted
        - [x] NoConflicts
        - [x] HostnameConflict
        - [x] ProtocolConflict
        - [x] ~~RouteConflict~~ *unused due to the spec's confusion between when to accept a route or not, we choose not to accept routes that don't match whatever routing support the listener has*
      - [x] Detached
        - [x] Attached
        - [x] ~~PortUnavailable~~ *unused, as the only time a port will be unavailable is if we can't schedule the Gateway due to host port binding, which will result in a gateway `Schedule` status of `NoResources`* 
        - [x] ~~UnsupportedExtension~~ *unused, not sure what the spec is referring to by "extensions" for listeners*
        - [x] UnsupportedProtocol *marked for any non-HTTP/HTTPS protocols for now*
        - [x] UnsupportedAddress *set if the user specified an address for the gateway*
      - [ ] Ready
        - [x] Ready
        - [x] Invalid *leveraged for anything that doesn't match spec guidelines, i.e. `HTTPS` protocol not specifying a TLS configuration*
        - [ ] Pending
      - [ ] ResolvedRefs
        - [x] ResolvedRefs
        - [x] InvalidCertificateRef
        - [x] InvalidRouteKinds
        - [ ] RefNotPermitted *this is unclear from the spec, talks about setting `RefNotPermitted` on the route in one place, and on the listener in another -- pretty sure it shouldn't be on the listener*
    - [x] Conditions
      - [x] Ready
        - [x] Ready
        - [x] ListenersNotValid
        - [x] ListenersNotReady
        - [x] AddressNotAssigned *set when someone specifies an address for the Gateway, since we don't support them*
      - [x] Scheduled
        - [x] NotReconciled
        - [x] NoResources
        - [x] *PodFailed* status added for when a deployment has crashed for some reason
        - [x] *Unknown* for any potentially unhandled Pod scheduling statuses
      - [x] *InSync* condition added to indicate synchronization with Consul
        - [x] *InSync* if our latest attempt to sync to Consul was successful
        - [x] *SyncError* if our latest attempt to sync failed, the condition message contains the sync error message

- [ ] HTTPRoute
  - [ ] Spec
    - [x] Hostnames
    - [ ] Rules
      - [x] Route matching
        - [x] Path-based matching
        - [x] Header-based matching
        - [x] Query-based matching
        - [x] Method-based matching
      - [ ] Filters
        - [x] Header modification
          - [x] Set headers
          - [x] Add headers
          - [x] Remove headers
        - [ ] Request mirroring
        - [ ] Request redirecting
        - [x] Extensions *not supported*
    - [ ] Backend Refs
      - [ ] Filters (see above)
      - [x] References
        - [x] Weights
        - [x] Group/Kind *only Kubernetes services with Consul entries supported for now*
        - [x] Name/Namespace lookups
        - [x] Port *check for existence, but ignored for Kubernetes services since we get our ports from Consul*
  - [x] Status
    - [x] Parent status updates on insertion
      - [x] Accepted *note that all of these are custom since the spec doesn't define reasons for the conditions*
        - [x] *Accepted* indicates route accepted by the gateway parent reference
        - [x] *InvalidRouteKind* rejected by the gateway because it doesn't match the listener allowed routes
        - [x] *ListenerNamespacePolicy* rejected by the gateway because it doesn't match the listener namespace policy
        - [x] *ListenerHostnameMismatch* rejected because of hostname mismatches between listener and route
        - [x] *BindError* generic error for things like label parsing issues
      - [x] ResolvedRefs *note that all of these are custom since the spec doesn't define reasons for the conditions*
        - [x] *ResolvedRefs* indicates that we were able to resolve all route references to things like Kubernetes/Consul services
        - [x] *ServiceNotFound* weren't able to find the referenced Kubernetes service
        - [x] *ConsulServiceNotFound* weren't able to find the referenced Consul mesh service

- [ ] TCPRoute - TODO
- [ ] TLSRoute - TODO
- [x] UDPRoute *not supported*