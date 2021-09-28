# Digitalocean + Cloudflare + Cert Manager + External DNS Setup

These are basic copy-paste commands for setting up a demo Kubernetes cluster that leverages
this project.

## Set environment variables

```bash
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_EMAIL=...
export CLOUDFLARE_ZONE=...
export DIGITALOCEAN_TOKEN=...
```

## Set up kubernetes cluster

```bash
doctl auth init -t $DIGITALOCEAN_TOKEN
doctl kubernetes cluster create gateway-controller-cluster --node-pool "name=worker-pool;size=s-2vcpu-2gb;count=1"
doctl registry create gateway
doctl kubernetes cluster registry add gateway-controller-cluster
doctl registry login gateway
doctl registry kubernetes-manifest | kubectl apply -f -
kubectl patch serviceaccount default -p '{"imagePullSecrets": [{"name": "registry-gateway"}]}'
```

## Install cert manager

```bash
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --version v1.5.3 --set installCRDs=true
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-api-token-secret
type: Opaque
stringData:
  api-token: $CLOUDFLARE_API_TOKEN
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: prod-issuer
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: account-key-prod
    email: $CLOUDFLARE_EMAIL
    solvers:
    - dns01:
        cloudflare:
          email: $CLOUDFLARE_EMAIL
          apiTokenSecretRef:
            name: cloudflare-api-token-secret
            key: api-token
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway
spec:
  secretName: gateway-production-certificate
  issuerRef:
    name: prod-issuer
  dnsNames:
  - gateway.$CLOUDFLARE_ZONE
EOF
```

## Install external dns

```bash
kubectl create namespace external-dns
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/external-dns/65a69275b1f76fa01b56a708d0514ae49edf30fd/docs/contributing/crd-source/crd-manifest.yaml
cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-dns
  namespace: external-dns
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: external-dns
  namespace: external-dns
rules:
- apiGroups: [""]
  resources: ["services","endpoints","pods"]
  verbs: ["get","watch","list"]
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["list", "watch"]
- apiGroups: ["externaldns.k8s.io"]
  resources: ["dnsendpoints"]
  verbs: ["get","watch","list"]
- apiGroups: ["externaldns.k8s.io"]
  resources: ["dnsendpoints/status"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: external-dns-viewer
  namespace: external-dns
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: external-dns
subjects:
- kind: ServiceAccount
  name: external-dns
  namespace: external-dns
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
  namespace: external-dns
spec:
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: external-dns
  template:
    metadata:
      labels:
        app: external-dns
    spec:
      serviceAccountName: external-dns
      containers:
      - name: external-dns
        image: k8s.gcr.io/external-dns/external-dns:v0.7.6
        args:
        - --source=crd
        - --provider=cloudflare
        - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
        - --crd-source-kind=DNSEndpoint
        env:
        - name: CF_API_TOKEN
          value: $CLOUDFLARE_API_TOKEN
        - name: CF_API_EMAIL
          value: $CLOUDFLARE_EMAIL
EOF
```

## Set up gateway crds

```bash
kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.0-rc1" | kubectl apply -f -
```

## Set up Consul

```bash
kubectl create namespace consul
helm repo add hashicorp https://helm.releases.hashicorp.com
cat <<EOF | helm install consul hashicorp/consul --namespace consul --version 0.33.0 -f -
global:
  name: consul
  image: "hashicorpdev/consul:581357c32"
  tls:
    enabled: true
    serverAdditionalDNSSANs:
    - consul-server.consul.svc.cluster.local
connectInject:
  enabled: true
controller:
  enabled: true
server:
  replicas: 1
  extraConfig: |
    {
      "log_level": "trace",
      "ports": {
        "grpc": 8502
      },
      "connect": {
        "enabled": true
      }
    }
EOF
kubectl patch statefulset -n consul consul-server --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/ports/-", "value": {"containerPort": 8502, "protocol": "TCP", "name": "grpc"}}]'
kubectl patch svc -n consul consul-server --type='json' -p='[{"op": "add", "path": "/spec/ports/-", "value": {"port": 8502, "targetPort": 8502, "protocol": "TCP", "name": "grpc"}}]'
kubectl get secret consul-ca-cert --namespace=consul -oyaml | grep -v '^\s*namespace:\s' | kubectl apply --namespace=default -f -
kubectl get secret consul-server-cert --namespace=consul -oyaml | grep -v '^\s*namespace:\s' | kubectl apply --namespace=default -f -
```

## Set up gateway controller

```bash
GOOS=linux go build
docker build -t registry.digitalocean.com/gateway/controller:1 .
docker push registry.digitalocean.com/gateway/controller:1
kubectl kustomize config | kubectl apply -f -
```

## Create gateway resources

Wait until all the above is stable and then:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: api-gateway.consul.hashicorp.com/v1alpha1
kind: GatewayClassConfig
metadata:
  name: test-gateway-class-config
spec:
  logLevel: trace
  serviceType: LoadBalancer
  image:
    consulAPIGateway: "registry.digitalocean.com/gateway/controller:1"
  consul:
    address: consul-server.consul.svc.cluster.local
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
  - protocol: HTTPS
    hostname: gateway.$CLOUDFLARE_ZONE
    port: 443
    name: https
    allowedRoutes:
      namespaces:
        from: Same
    tls:
      certificateRef:
        name: gateway-production-certificate
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

## Create external DNS entry

When the load balancer service has an external ip, then run.

```bash
export LB_IP=$(kubectl get svc test-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
cat <<EOF | kubectl apply -f -
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: gateway
  namespace: external-dns
spec:
  endpoints:
  - dnsName: gateway.$CLOUDFLARE_ZONE
    recordTTL: 180
    recordType: A
    targets:
    - $LB_IP
EOF
```

## Test

Once DNS has propagated

```bash
curl https://gateway.$CLOUDFLARE_ZONE
```

## Clean up resources

```bash
kubectl delete dnsendpoint gateway -n external-dns
kubectl delete httproute test-route
kubectl delete deployment echo
kubectl delete service echo
kubectl delete servicedefaults echo
kubectl delete gateway test-gateway
kubectl delete gatewayclass test-gateway-class
kubectl delete gatewayclassconfig test-gateway-class-config
doctl registry delete gateway
doctl kubernetes cluster delete gateway-controller-cluster
```