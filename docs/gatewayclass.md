
## GatewayClass (version: gateway.networking.k8s.io/v1alpha2, scope: Cluster)

A `GatewayClass` acts as a template for an individual Gateway deployment. In order
for the Consul API Gateway controller to know how to provision a Gateway, it must
reference a `GatewayClass` that has a controllerName value of `hashicorp.com/consul-api-gateway-controller`
and a parametersRef that references a valid `GatewayClassConfig` object.


### Fields
- `controllerName` - (type: `string`): This must be set to `hashicorp.com/consul-api-gateway-controller`.
- `description` - (type: `string`): Description helps describe a `GatewayClass` with more details.
- `parametersRef` - (type: `object`): ParametersRef is a reference to a resource that containsthe configuration parameters corresponding to the `GatewayClass`.This reference must correspond to a valid `GatewayClassConfig` object.
	- `group` - (type: `string`): Group is the group of the parameter reference. Must be set to `api-gateway.consul.hashicorp.com`.
	- `kind` - (type: `string`): Kind is kind of the parameter reference. Must be set to `GatewayClassConfig`.
	- `name` - (type: `string`): Name is the name of the parameter reference.