## UNRELEASED

## 0.3.0 (April 28, 2022)

IMPROVEMENTS:

* go: build with Go 1.18 [[GH-167](https://github.com/hashicorp/consul-api-gateway/issues/167)]


## 0.2.0 (April 27, 2022)

BREAKING CHANGES:

* Routes now require a [ReferencePolicy](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io%2fv1alpha2.ReferencePolicy) to permit references to services in other namespaces. [[GH-143](https://github.com/hashicorp/consul-api-gateway/issues/143)]

IMPROVEMENTS:

* changelog: add go-changelog templates and tooling [[GH-101](https://github.com/hashicorp/consul-api-gateway/issues/101)]
* k8s/controllers: watch for ReferencePolicy changes to reconcile and revalidate affected HTTPRoutes [[GH-156](https://github.com/hashicorp/consul-api-gateway/issues/156)]
* k8s/controllers: watch for ReferencePolicy changes to reconcile and revalidate affected TCPRoutes [[GH-162](https://github.com/hashicorp/consul-api-gateway/issues/162)]

BUG FIXES:

 * Apply namespace selector for allowed routes to the route's namespace instead of the route itself [[GH-119](https://github.com/hashicorp/consul-api-gateway/pull/119)]
 * Fix http route merging to make sure we merge routes that reference the same hostname [[GH-126](https://github.com/hashicorp/consul-api-gateway/pull/126)]

## 0.1.0 (February 23, 2022)

* Initial release of Consul API Gateway
  * Supported on Kubernetes /w Consul version 1.11.2 or greater
* Supports the following [Gateway API](https://gateway-api.sigs.k8s.io/) CRDs:
  * [v1alpha2.GatewayClass](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.Gateway)
  * [v1alpha2.Gateway](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClass)
  * [v1alpha2.HTTPRoute](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPRoute)
  * [v1alpha2.TCPRoute](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.TCPRoute)
