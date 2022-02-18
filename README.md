<h1>
  <img src="./assets/logo.svg" align="left" height="46px" alt="Consul logo"/>
  <span>Consul API Gateway</span>
</h1>

[![CI Status](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hashicorp/consul-api-gateway/actions/workflows/ci.yml?query=branch%3Amain) [![Discuss](https://img.shields.io/badge/discuss-consul--api--gateway-dc477d?logo=consul)](https://discuss.hashicorp.com/c/consul)

# Overview

The Consul API Gateway is a dedicated ingress solution for intelligently routing traffic to applications
running on a Consul Service Mesh. Currently it only runs on Kubernetes and is implemented as a
Kubernetes Gateway Controller but, in future releases, it will work across multiple scheduler and
runtime ecosystems.

Consul API Gateway implements the Kubernetes [Gateway API Specification](https://gateway-api.sigs.k8s.io/). This specification defines a set of custom resource definitions (CRD) that can create logical gateways and routes based on the path or protocol of a client request. Consul API Gateway solves two primary use cases:

- **Controlling access at the point of entry**: Consul API Gateway allows users to set the protocols of external connection requests and provide clients with TLS certificates from trusted providers (e.g., Verisign, Let’s Encrypt).
- **Simplifying traffic management**: The Consul API Gateway can load balance requests across services and route traffic to the appropriate service by matching one or more criteria, such as hostname, path, header presence or value, and HTTP Method type (e.g., GET, POST, PATCH).

## Prerequisites  

The Consul API Gateway must be installed on a Kubernetes cluster with the [Consul K8s](https://github.com/hashicorp/consul-k8s) service
mesh deployed on it. The installed version of Consul must be `v1.11.2` or greater.

The Consul Helm chart must be used, with specific settings, to install Consul on the Kubernetes
cluster. The Consul Helm chart must be version `0.40.0` or greater.  See the Consul API Gateway documentation for the required settings.

# Documentation

The primary documentation, including installation instructions, is available on the [Consul documentation website](https://www.consul.io/docs/api-gateway).

## Configuring and Deploying API Gateways

After Consul API Gateway has been installed, API Gateways are configured and deployed via the [Kubernetes Gateway API](https://github.com/kubernetes-sigs/gateway-api) standard. The [Kubernetes Gateway API webite](https://gateway-api.sigs.k8s.io/) explains the design of the standard, examples of how to
use it and the complete specification of the API.

The Consul API Gateway Beta supports the current version (`v1alpha2`) of the Gateway API.

**Supported Features:** Please see [Supported Features](./dev/docs/supported-features.md) for a list of K8s Gateway API features
supported by the current release of Consul API Gateway.

# Tutorial

We have a tutorial that walks you though installing and configuring Consul API Gateway to route traffic to multiple services of an example application. You can find it here: [Consul API Gateway tutorial](https://learn.hashicorp.com/tutorials/consul/kubernetes-api-gateway)

# Contributing

Thank you for your interest in contributing! Please refer to [CONTRIBUTING.md](https://github.com/hashicorp/consul-api-gateway/blob/main/.github/CONTRIBUTING.md#contributing) for guidance.

For development, please see our [Quick Start](./dev/docs/getting-started.md) guide. Other documentation can be found inside our [in-repo developer documentation](./dev/docs).
