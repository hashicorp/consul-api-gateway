
## GatewayClassConfig (version: api-gateway.consul.hashicorp.com/v1alpha1, scope: Cluster)

GatewayClassConfig describes the configuration of a consul-api-gateway GatewayClass.

### Fields
- `consul` - (type: `object`): Configuration information about connecting to Consul.
	- `address` - (type: `string`): The address of the consul server to communicate with in the gateway pod. If not specified, the pod will attempt to use a local agent on the host on which it is running.
	- `authentication` - (type: `object`): Consul authentication information
		- `account` - (type: `string`): The Kubernetes service account to authenticate as.
		- `managed` - (type: `boolean`): Whether deployments should be run with "managed" service accounts created by the gateway controller.
		- `method` - (type: `string`): The Consul auth method used for initial authentication by consul-api-gateway.
		- `namespace` - (type: `string`): The Consul namespace to use for authentication.
	- `ports` - (type: `object`): The information about Consul's ports
		- `grpc` - (type: `integer`): The grpc port for Consul's xDS server.
		- `http` - (type: `integer`): The port for Consul's HTTP server.
	- `scheme` - (type: `string`): The scheme to use for connecting to Consul.
- `copyAnnotations` - (type: `object`): Annotation Information to copy to services or deployments
	- `service` - (type: `array<string>`): List of annotations to copy to the gateway service.
- `image` - (type: `object`): Configuration information about the images to use
	- `consulAPIGateway` - (type: `string`): The image to use for consul-api-gateway.
	- `envoy` - (type: `string`): The image to use for Envoy.
- `logLevel` - (type: `string`): Logging levels
- `nodeSelector` - (type: `map<string, string>`): NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
- `serviceType` - (type: `string`): Service Type string describes ingress methods for a service
- `useHostPorts` - (type: `boolean`): If this is set, then the Envoy container ports are mapped to host ports.