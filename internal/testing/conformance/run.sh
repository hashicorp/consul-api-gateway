# Make sure the Kubernetes cluster that you're running against has Consul + Gateway API CRDs installed
rm -rf gateway-api
git clone --depth 1 git@github.com:kubernetes-sigs/gateway-api
cp kustomization.yaml proxydefaults.yaml gateway-api/conformance/
cd gateway-api/conformance/
kubectl kustomize ./ --output ./base/manifests.yaml
kubectl apply -f ./base/manifests.yaml

# TODO Right now, the conformance tests don't pass because they assume the route will work
#   as soon as the HTTPRoute has parent(s) listed; however, we sync resources into Consul/Envoy
#   and don't actually know when the route is available.
#   For now, we can only get the tests passing by installing the resources for a test and giving them
#   time to "settle" before running the corresponding test. This requires manual changes and can't
#   be automated in a sensible fashion.
go test ./ --gateway-class consul-api-gateway
