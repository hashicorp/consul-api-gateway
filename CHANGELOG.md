## UNRELEASED

BUG FIXES:

 * Apply namespace selector for allowed routes to the route's namespace instead of the route itself [[GH-119](https://github.com/hashicorp/consul-api-gateway/pull/119)]

## 0.1.0 (February 23, 2022)

* Initial release of Consul API Gateway
  * Supported on Kubernetes /w Consul version 1.11.2 or greater
* Supports the following [Gateway API](https://gateway-api.sigs.k8s.io/) CRDs:
  * [v1alpha2.GatewayClass](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.Gateway)
  * [v1alpha2.Gateway](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClass)
  * [v1alpha2.HTTPRoute](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPRoute)
  * [v1alpha2.TCPRoute](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.TCPRoute)
