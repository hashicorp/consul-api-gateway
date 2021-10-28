# Getting Started

[Table of Contents](./README.md)

## Quick Start (Mac)

This setup assumes using [Homebrew](https://brew.sh/) as a package manager and the [official HashiCorp tap](https://github.com/hashicorp/homebrew-tap). Consul will be installed in a Kubernetes cluster using the Helm chart, but the standalone binary is currently used for bootstrapping ACLs.

```bash
brew tap hashicorp/tap
brew cask install docker
brew install go jq kubectl kustomize kind helm hashicorp/tap/consul
```

Ensure Docker for Mac is running (enabling the Kubernetes single-node cluster is not necessary, as `kind` will build its own cluster), clone this repo, navigate to the root directory, then run:

```bash
./dev/run
```

Test out the Gateway controller:

```bash
kubectl apply -f dev/config/k8s/consul-api-gateway.yaml
```

Make sure that the echo container is routable:

```bash
curl https://localhost:8443 -k
```

You should expect to see output including a hostname and pod information - if the response if "no healthy upstream", the resources may not have finished being created yet.

Clean up the gateway you just created:

```bash
kubectl delete -f dev/config/k8s/consul-api-gateway.yaml
```
