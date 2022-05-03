
##  (version: gateway.networking.k8s.io/v1alpha2, scope: Namespaced)

ReferencePolicy identifies kinds of resources in other namespaces that are trusted to reference the specified kinds of resources in the same namespace as the policy. 
 Each ReferencePolicy can be used to represent a unique trust relationship. Additional Reference Policies can be used to add to the set of trusted sources of inbound references for the namespace they are defined within. 
 All cross-namespace references in Gateway API (with the exception of cross-namespace Gateway-route attachment) require a ReferencePolicy. 
 Support: Core

### Fields
- `from` - (type: `array<object>`): From describes the trusted namespaces and kinds that can reference the resources described in "To". Each entry in this list must be considered to be an additional place that references can be valid from, or to put this another way, entries must be combined using OR.  Support: Core
	- `group` - (type: `string`): Group is the group of the referent. When empty, the Kubernetes core API group is inferred.  Support: Core
	- `kind` - (type: `string`): Kind is the kind of the referent. Although implementations may support additional resources, the following Route types are part of the "Core" support level for this field:  * HTTPRoute * TCPRoute * TLSRoute * UDPRoute
	- `namespace` - (type: `string`): Namespace is the namespace of the referent.  Support: Core
- `to` - (type: `array<object>`): To describes the resources that may be referenced by the resources described in "From". Each entry in this list must be considered to be an additional place that references can be valid to, or to put this another way, entries must be combined using OR.  Support: Core
	- `group` - (type: `string`): Group is the group of the referent. When empty, the Kubernetes core API group is inferred.  Support: Core
	- `kind` - (type: `string`): Kind is the kind of the referent. Although implementations may support additional resources, the following types are part of the "Core" support level for this field:  * Service
	- `name` - (type: `string`): Name is the name of the referent. When unspecified, this policy refers to all resources of the specified Group and Kind in the local namespace.