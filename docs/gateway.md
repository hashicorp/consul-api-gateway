
## Gateway (version: gateway.networking.k8s.io/v1alpha2, scope: Namespaced)

A gateway represents an instance of service-traffic handling infrastructure. Gateways configure one or more listeners, which can bind to a set of IP address. Routes can then attach to a gateway listener to direct traffic from the gateway to a service.

### Fields
- `gatewayClassName` - (type: `string`): The name of a GatewayClass resource used for this Gateway.
- `listeners` - (type: `array<object>`): Listeners associated with this Gateway. Listeners definelogical endpoints that are bound on this Gateway's addresses. Atleast one listener MUST be specified. Each listener in a Gateway must have a unique combination of Hostname, Port, and Protocol.
	- `allowedRoutes` - (type: `object`): AllowedRoutes defines the types of routes thatMAY be attached to a Listener and the trusted namespaces wherethose Route resources MAY be present.
		- `namespaces` - (type: `object`): Namespaces indicates namespaces from whichRoutes may be attached to this Listener. This is restrictedto the namespace of this Gateway by default.
			- `from` - (type: `string`): From indicates where Routes will be selectedfor this Gateway."
			- `selector` - (type: `object`): Selector must be specified when From is set to "Selector". In that case, only Routes in Namespaces matching this Selector will be selected by this Gateway.
				- `matchExpressions` - (type: `array<object>`): matchExpressions is a list of labelselector requirements. The requirements are ANDed.
					- `key` - (type: `string`): key is the label key that theselector applies to.
					- `operator` - (type: `string`): operator represents a key's relationshipto a set of values. Valid operators areIn, NotIn, Exists and DoesNotExist.
					- `values` - (type: `array<string>`): values is an array of stringvalues. If the operator is In or NotIn,the values array must be non-empty. If theoperator is Exists or DoesNotExist, thevalues array must be empty. This array isreplaced during a strategic merge patch.
				- `matchLabels` - (type: `map<string, string>`): matchLabels is a map of {key,value}pairs. A single {key,value} in the matchLabelsmap is equivalent to an element of matchExpressions,whose key field is "key", the operator is "In",and the values array contains only "value". Therequirements are ANDed.
	- `hostname` - (type: `string`): Hostname specifies the virtual hostname to match for HTTP or HTTPS-based listeners. When unspecified,all hostnames are matched. This is implemented by checking the HTTP Host header sent on a client request.
	- `name` - (type: `string`): Name is the name of the Listener. This name MUST be unique within a Gateway.
	- `port` - (type: `integer`): Port is the network port of a listener.
	- `protocol` - (type: `string`): Protocol specifies the network protocol this listener expects to receive.
	- `tls` - (type: `object`): TLS is the TLS configuration for the Listener.This field is required if the Protocol field is "HTTPS".It is invalid to set this field if the Protocolfield is "HTTP" or "TCP".
		- `certificateRefs` - (type: `array<object>`): CertificateRefs contains a series of referencesto Kubernetes objects that contains TLS certificates andprivate keys. These certificates are used to establisha TLS handshake for requests that match the hostname ofthe associated listener. Each reference must be a KubernetesSecret, and, if using a Secret in a namespace other than theGateway's, must have a corresponding ReferencePolicy created.
			- `name` - (type: `string`): Name is the name of the Kubernetes Secret.
			- `namespace` - (type: `string`): Namespace is the namespace of the Secret. When unspecified, the local namespace is inferred.Note that when a namespace is specified, a ReferencePolicyobject is required in the specified namespace toallow that namespace's owner to accept the reference.
		- `mode` - (type: `string`): Mode defines the TLS behavior for the TLS session initiated by the client. The only supported mode at this time is `Terminate`
		- `options` - (type: `map<string, string>`): Options are a list of key/value pairs to enableextended TLS configuration for each implementation.