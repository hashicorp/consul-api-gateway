# Consul API Gateway [![CI Status](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml?query=branch%3Amain) [![Discuss](https://img.shields.io/badge/discuss-consul--api--gateway-dc477d?logo=consul)](https://discuss.hashicorp.com/c/consul)

# Overview

The Consul API Gateway is a dedicated ingress solution for intelligently routing traffic to applications
running on a Consul Service Mesh. Currently it only runs on Kubernetes and is implemented as a
Kubernetes Gateway Controller but, in future releases, it will work across multiple scheduler and
runtime ecosystems.

# Usage

## Prerequisites  

The Consul API Gateway must be installed on a Kubernetes cluster with the [Consul K8s](https://github.com/hashicorp/consul-k8s) service
mesh deployed on it. The installed version of Consul must be `v1.11-beta2` or greater.

The Consul Helm chart must be used, with specific settings, to install Consul on the Kubernetes
cluster. This can be done with the following commands:

```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
cat <<EOF | helm install consul hashicorp/consul --version 0.36.0 -f -
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

## Install the Tech Preview

To install the Consul API Gateway controller and a base Kubernetes `GatewayClass` that leverages the
API Gateway, run the following commands:

```bash
kubectl apply -k "github.com/hashicorp/consul-api-gateway/config/crd?ref=v0.1.0-techpreview"
kubectl apply -k "github.com/hashicorp/consul-api-gateway/config?ref=v0.1.0-techpreview"
```

You should now be able to deploy a Gateway by referencing the gateway class `default-consul-gateway-class` in
a Kubernetes `Gateway` manifest.

## Configuring and Deploying API Gateways

Consul API Gateways are configured and deployed via the [Kubernetes Gateway API](https://github.com/kubernetes-sigs/gateway-api) standard.
The [Kubernetes Gateway API webite](https://gateway-api.sigs.k8s.io/) explains the design of the standard, examples of how to
use it and the complete specification of the API. 

The Consul API Gateway Tech Preview supports current version (`v1alpha2`) of the Gateway API.

**Supported Features:** Please see [Supported Features](./dev/docs/supported-features.md) for a list of K8s Gateway API features
supported by the current release of Consul API Gateway.

# Tutorial

For an example of how to deploy a Consul API Gateway and use it alongside [CertManager](https://github.com/jetstack/cert-manager) and
[External DNS](https://github.com/kubernetes-sigs/external-dns), see the [Example Setup](./dev/docs/example-setup.md).


# Contributing

Thank you for your interest in contributing! Please refer to [CONTRIBUTING.md](https://github.com/hashicorp/consul-api-gateway/blob/main/.github/CONTRIBUTING.md#contributing) for guidance.

For development, please see our [Quick Start](./dev/docs/getting-started.md) guide. Other documentation can be found inside our [in-repo developer documentation](./dev/docs).
