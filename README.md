# Consul API Gateway [![CI Status](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml?query=branch%3Amain) [![Discuss](https://img.shields.io/badge/discuss-consul--api--gateway-dc477d?logo=consul)](https://discuss.hashicorp.com/c/consul)

# Overview

The Consul API Gateway implements a North/South managed gateway that integrates natively with the Consul Service Mesh. Currently this
is implemented as a Kubernetes Gateway Controller, but is meant to eventually work across multiple scheduler and runtime ecosystems.

# Usage

The Consul API Gateway project Kubernetes integration leverages connect-injected services managed by the
[Consul K8s](https://github.com/hashicorp/consul-k8s) project. To use this project, make sure you have a running Kubernetes cluster and
Consul 1.11 or greater installed [via Helm](https://github.com/hashicorp/consul-k8s#usage) with Connect injection support enabled.

Our default `kustomization` manifests also assume that the Consul helm chart has TLS enabled. To install a compatible Consul instance via
Helm, you can run the following commands:

```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
cat <<EOF | helm install consul hashicorp/consul --version 0.35.0 -f -
global:
  name: consul
  image: "hashicorp/consul:1.11.0-beta2"
  tls:
    enabled: true
connectInject:
  enabled: true
controller:
  enabled: true
EOF
```

To install the gateway controller and a base Kubernetes `GatewayClass` that leverages the API Gateway, run the following commands:

```bash
kubectl apply -k "github.com/hashicorp/consul-api-gateway/config/crd?ref=v0.1.0-techpreview"
kubectl apply -k "github.com/hashicorp/consul-api-gateway/config?ref=v0.1.0-techpreview"
```

You should now be able to deploy a Gateway by referencing the gateway class `default-consul-gateway-class` in a Kubernetes `Gateway`
manifest.

For more detailed instructions and an example of how to use this alongside
[CertManager](https://github.com/jetstack/cert-manager) and [External DNS](https://github.com/kubernetes-sigs/external-dns) see the
[development documentation](./dev/docs/example-setup.md).

# Tutorials

For development, please see our [Quick Start](./dev/docs/getting-started.md) guide. Other documentation can be found inside our [in-repo developer documentation](./dev/docs).
