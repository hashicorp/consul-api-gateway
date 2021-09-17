# Quick Start

Install Docker for Mac. Execute the following:

```/bin/bash
brew install kubectl kind helm consul jq go
./scripts/develop
```

Test out the Gateway controller:

```/bin/bash
cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: test-gateway
  annotations:
    "api-gateway.consul.hashicorp.com/image": "consul-api-gateway:1"
    "api-gateway.consul.hashicorp.com/consul-http-address": "host.docker.internal"
    "api-gateway.consul.hashicorp.com/consul-http-port": "443"
    "api-gateway.consul.hashicorp.com/consul-auth-method": "consul-api-gateway"
    "api-gateway.consul.hashicorp.com/service-account": "consul-api-gateway"
spec:
  gatewayClassName: consul-api-gateway
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
```
