# Conformance Testing

The resources here facilitate the running of the conformance tests defined upstream in [kubernetes-sigs/gateway-api](https://github.com/kubernetes-sigs/gateway-api).

## Special Considerations

The framework defines its own set of Kubernetes resources using kustomization yaml. These should generally work with any implementation;
however, we currently have to make a few patches in order for things to work with Consul. Our goal long-term is to remove the
need for these patches.

- Consul isn't, by default, aware of each of the created services. To make this work, we patch the `connect-inject` annotation onto
    each `Deployment`'s template.
- The `Deployments` defined upstream do not specify a `containerPort` in the `Pod` template. Consul relies on this `containerPort`
    when a `connect-service-port` annotation is not present. To make this work, we patch the `connect-service-port` annotation onto
    each `Deployment`'s template. They all use the same port.
- The Consul services default to a protocol of `tcp`; however, the testing framework uses `http`. To make this work, we create
    a `ProxyDefaults` resource which sets the protocol to `http` globally.

## Status

The conformance tests cannot currently run in an automated fashion. They are not included in our CI yet.

Due to the controller not currently knowing when Consul/Envoy are ready after syncing in new routes, the route appears
"ready" to the conformance testing framework before the gateway can actually respond to requests for the route. The
framework then sends HTTP requests as soon as the route appears ready and the request is rejected with an error like
the following:

```log
Get "http://35.229.22.36": dial tcp 35.229.22.36:80: connect: connection refused
```

This doesn't mean we cannot run the conformance tests, they just have to be run manually, one at a time.  
To run a particular conformance test, you need to:

1. Create a GKE cluster (or any other standard Kubernetes cluster) and install Consul + Consul API Gateway.
     The [usage docs](https://www.consul.io/docs/api-gateway/api-gateway-usage#installation) explain how to do this.

2. clone the [kubernetes-sigs/gateway-api](https://github.com/kubernetes-sigs/gateway-api)
repo and copy our patches into the `conformance` subdirectory:

    ```shell
    git clone --depth 1 git@github.com:kubernetes-sigs/gateway-api
    cp kustomization.yaml proxydefaults.yaml gateway-api/conformance/
    ```

3. make your way into the `conformance` directory, then patch and install the base resources:

    ```shell
    cd gateway-api/conformance/
    kubectl kustomize ./ --output ./base/manifests.yaml
    kubectl apply -f manifests.yaml --validate=false
    ```

4. install the test-specific resources (adjust name appropriately):

    ```shell
    kubectl apply -f tests/httproute-matching.yaml
    ```

5. modify the last line of `conformance_test.go` that passes the list of tests to include only the test that you want to run:

    ```go
    cSuite.Run(t, []suite.ConformanceTest{tests.HTTPRouteMatchingAcrossRoutes})
    ```

6. run the test:
    ```shell
    go test ./ --gateway-class consul-api-gateway --cleanup=0
    ```

7. repeat steps 4-6 for other tests
