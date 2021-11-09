# Consul API Gateway ![CI Status](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml/badge.svg) [![Discuss](https://img.shields.io/badge/discuss-consul--api--gateway-ca2171)](https://discuss.hashicorp.com/c/consul)

# Overview

The Consul API Gateway implements a North/South managed gateway that integrates natively with the Consul Service Mesh. Currently this
is implemented as a Kubernetes Gateway Controller, but is meant to eventually work across multiple scheduler and runtime ecosystems.

# Usage

The Consul API Gateway project Kubernetes integration leverages connect-injected services managed by the
[Consul K8s](https://github.com/hashicorp/consul-k8s) project. To use this project, make sure you have a running Kubernetes cluster and
Consul 1.11 or greater installed [via Helm](https://github.com/hashicorp/consul-k8s#usage) with Connect injection support enabled.

To install the gateway controller and a base Kubernetes `GatewayClass` that leverages the API Gateway, run the following commands:

```bash
kubectl kustomize "github.com/hashicorp/consul-api/gateway/config/crd?ref=v0.1.0" --reorder=none | kubectl apply -f -
kubectl kustomize "github.com/hashicorp/consul-api/gateway/config?ref=v0.1.0" --reorder=none | kubectl apply -f
```

You should now be able to deploy a Gateway by referencing the gateway class `default-consul-gateway-class` in a Kubernetes `Gateway`
manifest.

For more detailed instructions and an example of how to use this alongside
[CertManager](https://github.com/jetstack/cert-manager) and [External DNS](https://github.com/kubernetes-sigs/external-dns) see the
[development documentation](./dev/docs/example-setup.md).

# Tutorials

For development, please see our [Quick Start](./dev/docs/getting-started.md) guide. Other documentation can be found inside our [in-repo developer documentation](./dev/docs).
