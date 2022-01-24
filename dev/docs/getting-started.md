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

## Deploying a custom Docker image on a development cluster

- Create a Docker image from your local branch with `make docker`

- *Optional*: Some k8s setups require loading a custom image into the runtime environment for it to get pulled by worker nodes. For a local kind cluster you can load the image using the subcommand [`kind load docker-image`](https://kind.sigs.k8s.io/docs/user/quick-start/#loading-an-image-into-your-cluster)

- In `config/deployment/deployment.yaml`, edit `image` to point to the name of the image you uploaded to the cluster.

- Apply the version of the CRDs and Consul API Gateway deployment config from your local branch.
```
kubectl apply -k config/crd
kubectl apply -k config
```

## Running tests

`go test ./...` will run the default set of tests. Note that some of these tests use the [`consul/sdk`](https://github.com/hashicorp/consul/tree/main/sdk) test helper package, which shells out to the Consul binary on your `$PATH`. You'll want to ensure `which consul` and `consul -v` are pointing to the Consul binary you expect - either a sufficiently recent version, or a custom build if your feature work requires upstream changes in Consul core.

#### End-to-end tests

The end-to-end test suite uses [kind](https://kind.sigs.k8s.io/) to spin up a local Kubernetes cluster, deploy the Consul API Gateway controller, and check that gateways and routes are created, attached and succesfully routable. These tests can be included when running the test suite by passing a tag, `go test ./... -tags e2e`
