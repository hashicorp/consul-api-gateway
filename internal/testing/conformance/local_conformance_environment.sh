#!/bin/sh

#test suite assumes that kind cluster is already spun up with metal lb installed
#this step will initiate the cluster set up piece locally, but won't automatically
#tear down kind cluster

testID=$RANDOM
clusterName="consul-api-gateway-conformance-test-$testID"
imageName="consul-api-gateway-$testID"
kind create cluster --name $clusterName

#build image and load into cluster
docker build --tag $imageName --file ../../../Dockerfile.local ../../../.
kind load docker-image $imageName $imageName --name $clusterName
