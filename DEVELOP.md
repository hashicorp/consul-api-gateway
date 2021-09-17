# Quick Start

Install Docker for Mac. Execute the following:

```/bin/bash
brew install kubectl kind helm consul jq go
./scripts/develop
```

Test out the Gateway controller:

```/bin/bash
cat <<EOF | kubectl apply -f -
apiVersion: api-gateway.consul.hashicorp.com/v1alpha1
kind: GatewayClassConfig
metadata:
  name: test-gateway-class-config
spec:
  image:
    consulAPIGateway: "consul-api-gateway:1"
  consul:
    address: "host.docker.internal"
    scheme: https
    caSecret: consul-ca-cert
    ports:
      http: 443
    authentication:
      account: consul-api-gateway
      method: consul-api-gateway
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: GatewayClass
metadata:
  name: test-gateway-class
spec:
  controller: "hashicorp.com/consul-api-gateway-controller"
  parametersRef:
    group: api-gateway.consul.hashicorp.com
    kind: GatewayClassConfig
    name: test-gateway-class-config
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: test-gateway
spec:
  gatewayClassName: test-gateway-class
  listeners:
  - protocol: HTTP
    port: 8083
    name: my-http
    allowedRoutes:
      namespaces:
        from: Same
  - protocol: HTTPS
    port: 8443
    name: my-https
    allowedRoutes:
      namespaces:
        from: Same
    tls:
      certificateRef:
        name: consul-server-cert
EOF
```

Clean up the gateway you just created:

```
kubectl delete gateway test-gateway
kubectl delete gatewayclass test-gateway-class
kubectl delete gatewayclassconfig test-gateway-class-config
```
