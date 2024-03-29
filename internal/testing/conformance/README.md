# Conformance Testing

The resources here facilitate the running of the conformance tests defined upstream in [kubernetes-sigs/gateway-api](https://github.com/kubernetes-sigs/gateway-api).

## Special Considerations

The framework defines its own set of Kubernetes resources using kustomization yaml. These should generally work with any implementation;
however, we currently have to make a few patches in order for things to work with Consul. Our goal long-term is to remove the
need for these patches.

- The `Deployments` defined upstream do not specify a `containerPort` in the `Pod` template. Consul relies on this `containerPort`
    when a `connect-service-port` annotation is not present. To make this work, we patch the `connect-service-port` annotation onto
    each `Deployment`'s template. They all use the same port.
- The Consul services default to a protocol of `tcp`; however, the testing framework uses `http`. To make this work, we create
    a `ProxyDefaults` resource which sets the protocol to `http` globally.
- GitHub Actions' default hosted runner is not powerful enough to run all pods specified upstream in kind.
    To cope with this, we reduce all `Deployments` to 1 replica.

## Status

The conformance tests are run nightly in GitHub Actions using the workflow [here](/.github/workflows/conformance.yml).
You may also run the workflow on demand from this repo's Actions tab, by following the `conformance/*` branch naming convention, or by adding the `pr/run-conformance` label to your pull request.
