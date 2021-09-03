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
    "polar.hashicorp.com/use-host-ports": "true"
    "polar.hashicorp.com/consul-http-address": "host.docker.internal"
    "polar.hashicorp.com/consul-http-port": "443"
    "polar.hashicorp.com/image": "polar:1"
    "polar.hashicorp.com/auth-method": "polar"
    "polar.hashicorp.com/service-account": "polar"
    "polar.hashicorp.com/envoy": "polar:1"
spec:
  gatewayClassName: polar
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
EOF
```

You should be able to hit the gateway from your local host:

```
curl localhost:8083
curl localhost:8443
```

Clean up the gateway you just created:

```
kubectl delete gateway test-gateway
```