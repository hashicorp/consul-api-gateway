#!/bin/sh -x

# This image needs to be accessible to the k8s cluster that you're running against
CAPIGW_IMAGE="gcr.io/hc-251fc71211c2420b9483551bf71/consul-api-gateway-oss:5"

# Install CRDs
kubectl apply --kustomize="github.com/hashicorp/consul-api-gateway/config/crd?ref=v0.1.0"

# Install Consul using main branch of consul-k8s
rm -rf consul-k8s
git clone --depth=1 git@github.com:hashicorp/consul-k8s.git
helm install --values ./consul-config.yaml consul --set apiGateway.image=$CAPIGW_IMAGE ./consul-k8s/charts/consul --create-namespace --namespace=consul
kubectl wait --for=condition=Ready --timeout=300s --namespace=consul pods --all

# Pull down gateway-api branch supporting eventual consistency
rm -rf gateway-api
git clone --depth 1 --branch eventually-consistent-conformance git@github.com:nathancoleman/gateway-api.git

# Patch base resources to include Consul-specific requirements
cp kustomization.yaml proxydefaults.yaml ./gateway-api/conformance/
cd ./gateway-api/conformance/
kubectl kustomize ./ --output ./base/manifests.yaml
kubectl apply -f ./base/manifests.yaml

# Run conformance tests
go test ./ --gateway-class consul-api-gateway --cleanup=0
