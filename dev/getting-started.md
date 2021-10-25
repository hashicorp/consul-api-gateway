# Getting Started

[Table of Contents](./README.md)

## Quick Start (Mac)

This setup assumes using [Homebrew](https://brew.sh/) as a package manager and the [official HashiCorp tap](https://github.com/hashicorp/homebrew-tap).

```bash
brew tap hashicorp/tap
brew cask install docker
brew install go jq kubectl kustomize kind helm hashicorp/tap/consul
```

Open Docker for Mac, go to Preferences, enable the Kubernetes single-node cluster and wait for it to start. Clone this repo, navigate into the directory, then run:

```bash
./scripts/develop
```

Test out the Gateway controller:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: api-gateway.consul.hashicorp.com/v1alpha1
kind: GatewayClassConfig
metadata:
  name: test-gateway-class-config
spec:
  useHostPorts: true
  logLevel: trace
  image:
    consulAPIGateway: "consul-api-gateway:1"
  consul:
    scheme: https
    caSecret: consul-ca-cert
    ports:
      http: 8501
      grpc: 8502
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: GatewayClass
metadata:
  name: test-gateway-class
spec:
  controllerName: "hashicorp.com/consul-api-gateway-controller"
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
  - protocol: HTTPS
    hostname: localhost
    port: 8443
    name: https
    allowedRoutes:
      namespaces:
        from: Same
    tls:
      certificateRefs:
        - name: consul-server-cert
---
apiVersion: consul.hashicorp.com/v1alpha1
kind: ServiceDefaults
metadata:
  name: echo
spec:
  protocol: http
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: echo
  name: echo
spec:
  ports:
  - port: 8080
    name: high
    protocol: TCP
    targetPort: 8080
  selector:
    app: echo
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: echo
  name: echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
  template:
    metadata:
      labels:
        app: echo
      annotations:
        'consul.hashicorp.com/connect-inject': 'true'
    spec:
      containers:
      - image: gcr.io/kubernetes-e2e-test-images/echoserver:2.2
        name: echo
        ports:
        - containerPort: 8080
        env:
          - name: NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: POD_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: test-route
spec:
  parentRefs:
  - name: test-gateway
  rules:
  - backendRefs:
    - kind: Service
      name: echo
      port: 8080
EOF
```

Make sure that the echo container is routable:

```bash
curl https://localhost:8443 -k
```

Clean up the gateway you just created:

```bash
kubectl delete httproute test-route
kubectl delete deployment echo
kubectl delete service echo
kubectl delete servicedefaults echo
kubectl delete gateway test-gateway
kubectl delete gatewayclass test-gateway-class
kubectl delete gatewayclassconfig test-gateway-class-config
```