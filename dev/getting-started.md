# Getting Started

[Table of Contents](./README.md)

## Quick Start (Mac)

This setup assumes using [Homebrew](https://brew.sh/) as a package manager. Consul will be installed in a Kubernetes cluster using the Helm chart, so it's not necessary to install separately.

```bash
brew cask install docker
brew install go jq kubectl kustomize kind helm
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

TODO: You should expect...

Clean up the gateway you just created:

```bash
kubectl delete -f dev/config/k8s/consul-api-gateway.yaml
```
