
## GatewayClass (version: gateway.networking.k8s.io/v1alpha2, scope: Cluster)

GatewayClass describes a class of Gateways available to the user for creating Gateway resources. 
 It is recommended that this resource be used as a template for Gateways. This means that a Gateway is based on the state of the GatewayClass at the time it was created and changes to the GatewayClass or associated parameters are not propagated down to existing Gateways. This recommendation is intended to limit the blast radius of changes to GatewayClass or associated parameters. If implementations choose to propagate GatewayClass changes to existing Gateways, that MUST be clearly documented by the implementation. 
 Whenever one or more Gateways are using a GatewayClass, implementations MUST add the `gateway-exists-finalizer.gateway.networking.k8s.io` finalizer on the associated GatewayClass. This ensures that a GatewayClass associated with a Gateway is not deleted while in use. 
 GatewayClass is a Cluster level resource.

### Fields
- `controllerName` - (type: `string`): ControllerName is the name of the controller that is managing Gateways of this class. The value of this field MUST be a domain prefixed path.  Example: "example.net/gateway-controller".  This field is not mutable and cannot be empty.  Support: Core
- `description` - (type: `string`): Description helps describe a GatewayClass with more details.
- `parametersRef` - (type: `object`): ParametersRef is a reference to a resource that contains the configuration parameters corresponding to the GatewayClass. This is optional if the controller does not require any additional configuration.  ParametersRef can reference a standard Kubernetes resource, i.e. ConfigMap, or an implementation-specific custom resource. The resource can be cluster-scoped or namespace-scoped.  If the referent cannot be found, the GatewayClass's "InvalidParameters" status condition will be true.  Support: Custom
	- `group` - (type: `string`): Group is the group of the referent.
	- `kind` - (type: `string`): Kind is kind of the referent.
	- `name` - (type: `string`): Name is the name of the referent.
	- `namespace` - (type: `string`): Namespace is the namespace of the referent. This field is required when referring to a Namespace-scoped resource and MUST be unset when referring to a Cluster-scoped resource.