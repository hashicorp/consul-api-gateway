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
  name: polar-test
  annotations:
    "polar.hashicorp.com/consul-http-address": "host.docker.internal:443"
    "polar.hashicorp.com/image": "polar:1"
    "polar.hashicorp.com/auth-method": "polar"
    "polar.hashicorp.com/service-account": "polar"
    "polar.hashicorp.com/envoy": "polar:1"
spec:
  gatewayClassName: polar
  listeners:  
  - protocol: HTTP
    port: 80
    name: http
    allowedRoutes:
      namespaces:
        from: Same
EOF
```