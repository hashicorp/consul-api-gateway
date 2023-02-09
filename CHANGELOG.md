## UNRELEASED

BUG FIXES:

* Fix `cross-namespace-policy` not being applied to namespaces created by the controller. [[GH-](https://github.com/hashicorp/consul-api-gateway/pull/)]

## 0.5.0 (November 17, 2022)

FEATURES:

* Switch deployed gateways to use TTL-based health checks to better support running with Consul servers that are not on the same network as a gateway [[GH-371](https://github.com/hashicorp/consul-api-gateway/issues/371)]

IMPROVEMENTS:

* Add optional `consul.partition` and `consul.serverName` to GatewayClassConfig CRD. If set these will be used to initialize the partition and server name used in TLS verification for communicating with Consul in a deployment. [[GH-450](https://github.com/hashicorp/consul-api-gateway/issues/450)]
* Add optional `podSecurityPolicy` to GatewayClassConfig CRD. If set and "managed" ServiceAccounts are being used, a Role and RoleBinding are created to attach the named `PodSecurityPolicy` to the managed ServiceAccount. [[GH-433](https://github.com/hashicorp/consul-api-gateway/issues/433)]
* Add optional configuration for maximum upstream connections to GatewayClassConfig CRD. If unset, behavior is unchanged and Envoy's default will be used. [[GH-405](https://github.com/hashicorp/consul-api-gateway/issues/405)]
* Add support for tolerations to Consul API Gateway Controller and GatewayClassConfig. [[GH-426](https://github.com/hashicorp/consul-api-gateway/issues/426)]
* Integrate consul-server-connection-manager to support Agentless consul server discovery [[GH-449](https://github.com/hashicorp/consul-api-gateway/issues/449)]
* Support distroless Envoy images (with continued support for distroful images) [[GH-391](https://github.com/hashicorp/consul-api-gateway/issues/391)]
* api: add OpenAPI schema and stubs for bootstrap token CRUD [[GH-384](https://github.com/hashicorp/consul-api-gateway/issues/384)]
* go: update to Go v1.19 [[GH-424](https://github.com/hashicorp/consul-api-gateway/issues/424)]
* makefile: switch back to upstream go-changelog repo [[GH-385](https://github.com/hashicorp/consul-api-gateway/issues/385)]

BUG FIXES:

* Delete gateway ACL tokens on shutdown so they are not orphaned after being provisioned at startup. [[GH-377](https://github.com/hashicorp/consul-api-gateway/issues/377)]
* Fix failing root certificate watch for controller when deployed in secondary federated datacenter. [[GH-368](https://github.com/hashicorp/consul-api-gateway/issues/368)]
* When a gateway is created in a namespace that doesn't exist in Consul and namespace mirroring is enabled, create the namespace in Consul. [[GH-397](https://github.com/hashicorp/consul-api-gateway/issues/397)]

NOTES:

* RefNotPermitted error is now returned instead of InvalidCertificateRef in the case where a cross namespace certificate is not allowed by a ReferenceGrant [[GH-412](https://github.com/hashicorp/consul-api-gateway/issues/412)]

## 0.4.0 (August 16, 2022)
DEPRECATIONS:

* gateway-api: ReferencePolicy is deprecated and will be removed in a future release. The functionally identical ReferenceGrant should be used instead. [[GH-224](https://github.com/hashicorp/consul-api-gateway/issues/224)]

FEATURES:

* Assign BackendNotFound reason to ResolvedRefs condition on routes where the backend reference is a supported kind but does not exist [[GH-291](https://github.com/hashicorp/consul-api-gateway/issues/291)]
* Assign InvalidKind reason to ResolvedRefs condition on routes where the backend reference is an unknown or unsupported kind [[GH-290](https://github.com/hashicorp/consul-api-gateway/issues/290)]
* Support prefix replacement URLRewrite filter ([docs](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPPathModifier)) [[GH-282](https://github.com/hashicorp/consul-api-gateway/issues/282)]
* gateway-api: update to the [v0.5.0-rc1](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.5.0-rc1) release with v1beta1 resource support [[GH-224](https://github.com/hashicorp/consul-api-gateway/issues/224)]
* gateway-api: update to the [v0.5.0-rc2](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.5.0-rc2) release with v1beta1 resource support [[GH-279](https://github.com/hashicorp/consul-api-gateway/issues/279)]
* gateway-api: update to the [v0.5.0](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.5.0) release with v1beta1 resource support [[GH-283](https://github.com/hashicorp/consul-api-gateway/issues/283)]

BUG FIXES:

* Fix intentions syncing for multiple gateways bound to a single route. [[GH-308](https://github.com/hashicorp/consul-api-gateway/issues/308)]
* Revalidate HTTPRoutes and TCPRoutes and update status when the Kubernetes Service(s) that they reference are modified [[GH-247](https://github.com/hashicorp/consul-api-gateway/issues/247)]
* Sync in-memory store to Consul at a regular interval in the background [[GH-278](https://github.com/hashicorp/consul-api-gateway/issues/278)]

## 0.3.0 (June 21, 2022)
BREAKING CHANGES:

* Gateway listener `certificateRefs` to secrets in a different namespace now require a [ReferencePolicy](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io%2fv1alpha2.ReferencePolicy) [[GH-154](https://github.com/hashicorp/consul-api-gateway/issues/154)]

FEATURES:

* Added a new configuration option called deployment to GatewayClassConfig that allows the user to configure the number of instances that are deployed per gateway. [[GH-195](https://github.com/hashicorp/consul-api-gateway/issues/195)]
* Define anti-affinity rules so that the scheduler will attempt to evenly spread gateway pods across all available nodes [[GH-202](https://github.com/hashicorp/consul-api-gateway/issues/202)]

IMPROVEMENTS:

* go: build with Go 1.18 [[GH-167](https://github.com/hashicorp/consul-api-gateway/issues/167)]
* k8s/controllers: watch for ReferencePolicy changes to reconcile and revalidate affected Gateways [[GH-207](https://github.com/hashicorp/consul-api-gateway/issues/207)]

BUG FIXES:

* Clean up stale routes from gateway listeners when not able or allowed to bind, to prevent serving traffic for a detached route. [[GH-197](https://github.com/hashicorp/consul-api-gateway/issues/197)]
* Clean up stale routes from gateway listeners when route no longer references the gateway. [[GH-200](https://github.com/hashicorp/consul-api-gateway/issues/200)]
* Fix SPIFFE validation for connect certificates that have no URL (e.g., Vault connect certificates) [[GH-225](https://github.com/hashicorp/consul-api-gateway/issues/225)]
* Properly handle re-registration of deployed gateways when an agent no longer has the gateway in its catalog [[GH-227](https://github.com/hashicorp/consul-api-gateway/issues/227)]

NOTES:

* Gateway IP address assignment logic updated to include the case when multiple different pod IPs exist [[GH-201](https://github.com/hashicorp/consul-api-gateway/issues/201)]

## 0.2.1 (April 29, 2022)

BUG FIXES:

* k8s/reconciler: gateway addresses have invalid empty string when LoadBalancer services use a hostname for ExternalIP (like EKS) [[GH-187](https://github.com/hashicorp/consul-api-gateway/issues/187)]

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
