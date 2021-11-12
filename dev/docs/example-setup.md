# Digitalocean + Cloudflare + CertManager + External DNS Setup

This is a guide for setting up a full demo environment using DigitalOcean hosted Kubernetes and integrating
a Consul API Gateway with CertManager and ExternalDNS backed by Cloudflare.

Before you begin, make sure you have:

1. A Cloudflare account and API token for a registered domain you own.
2. A DigitalOcean account and API token.

## Installing dependencies

This tutorial leverages the DigitalOcean command-line utility, `jq`, `helm`, and `kubectl`.

### MacOS X

```bash
brew install jq helm kubectl doctl
```

## Setting up a Kubernetes cluster

First we'll set up a Kubernetes cluster using the `doctl` utility. Export your DigitalOcean
API token as an environment variable.

```bash
export DIGITALOCEAN_TOKEN=...
```

Next authenticate your command line binary and create a Kubernetes cluster named `demo-cluster`
using the following commands.

```bash
doctl auth init -t $DIGITALOCEAN_TOKEN
doctl kubernetes cluster create demo-cluster --node-pool "name=worker-pool;size=s-2vcpu-2gb;count=1"
```

## Installing Consul and the Consul API Gateway

The Consul API Gateway relies on an existing Consul deployment, so in this next
step we'll deploy a Consul cluster to our new Kubernetes environment. We'll also
add the ability to transparently inject Consul Connect to containers to automatically
add them to our service mesh.

### Set up Consul

We'll need to enable the HashiCorp Helm repo and install the latest Consul chart.

```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
cat <<EOF | helm install consul hashicorp/consul --version 0.36.0 -f -
global:
  name: consul
  image: "hashicorp/consul:1.11.0-beta2"
  tls:
    enabled: true
connectInject:
  enabled: true
controller:
  enabled: true
server:
  replicas: 1
EOF
```

If `helm` is having problems finding the proper version of the chart, ensure that
the local repositories are up-to-date by running `helm repo update`.

### Set up gateway controller

We have provided a set of `kustomize` manifests for installing the Consul API Gateway controller and CRDs.
Apply them to your cluster using the following commands.

```bash
kubectl apply -k "github.com/hashicorp/consul-api-gateway/config/crd?ref=v0.1.0-techpreview"
kubectl apply -k "github.com/hashicorp/consul-api-gateway/config?ref=v0.1.0-techpreview"
```

## Installing the demo Gateway and Mesh Service

Now that we have our controller set up and a default `GatewayClass` installed, we'll deploy
a full `Gateway` instance and hook up a Kubernetes service that is registered with Consul and
on the service mesh.

Because we're demonstrating how the Consul API Gateway interacts with CertManager and we want
the hosts to be publically routable, you'll need to export the following environment variables.

```bash
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_EMAIL=...
export DNS_HOSTNAME=...
```

### Install third-party dependencies and demo

First we'll install `cert-manager` from the published `helm` repo.

```bash
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --version v1.5.3 --set installCRDs=true
```

If `helm` is having problems finding the proper version of the chart, ensure that
the local repositories are up-to-date by running `helm repo update`.

### **Option 1 (recommended)**: Deploy the Demo via Kustomize

We have provided example manifests that can be used with `kustomize` to deploy the demo.
In order to configure them, you'll need to patch the placeholder variables in the deployment
with the following commands:

```bash
mkdir -p demo-deployment/example
cat <<EOF > demo-deployment/example/kustomization.yaml 
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- github.com/hashicorp/consul-api-gateway/config/example?ref=v0.1.0-techpreview

patches:
- target:
    group: gateway.networking.k8s.io
    version: v1alpha2
    kind: Gateway
    name: example-gateway
  patch: |-
    - op: replace
      path: /spec/listeners/0/hostname
      value: $DNS_HOSTNAME
    - op: replace
      path: /metadata/annotations/external-dns.alpha.kubernetes.io~1hostname
      value: $DNS_HOSTNAME
- target:
    group: apps
    version: v1
    kind: Deployment
    name: external-dns
  patch: |-
    - op: replace
      path: /spec/template/spec/containers/0/env/0/value
      value: $CLOUDFLARE_API_TOKEN
    - op: replace
      path: /spec/template/spec/containers/0/env/1/value
      value: $CLOUDFLARE_EMAIL
- target:
    version: v1
    kind: Secret
    name: cloudflare-api-token-secret
  patch: |-
    - op: replace
      path: /stringData/api-token
      value: $CLOUDFLARE_API_TOKEN
- target:
    group: cert-manager.io
    version: v1
    kind: Issuer
    name: prod-issuer
  patch: |-
    - op: replace
      path: /spec/acme/email
      value: $CLOUDFLARE_EMAIL
    - op: replace
      path: /spec/acme/solvers/0/dns01/cloudflare/email
      value: $CLOUDFLARE_EMAIL
- target:
    group: cert-manager.io
    version: v1
    kind: Certificate
    name: gateway
  patch: |-
    - op: replace
      path: /spec/dnsNames/0
      value: $DNS_HOSTNAME
EOF
kubectl apply -k demo-deployment/example
```

### Option 2: Manually apply the demo configurations

If you followed the steps for Option 1, you may skip this section.

#### Creating a CertManager Issuer and Provisioning a certificate

First we'll create a CertManager `Issuer` and `Certificate` for use in our `Gateway` instance
and hook it up to Let's Encrypt via ACME DNS challenges.

```bash
cat <<EOF | kubectl apply -f -
---
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
  - $DNS_HOSTNAME
EOF
```

#### Installing ExternalDNS configured for CloudFlare

Next we'll create service accounts and deploy ExternalDNS hooked up to CloudFlare.

```bash
cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-dns
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: external-dns
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
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: external-dns
subjects:
- kind: ServiceAccount
  name: external-dns
  namespace: default
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
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
        - --source=service
        - --provider=cloudflare
        env:
        - name: CF_API_TOKEN
          value: $CLOUDFLARE_API_TOKEN
        - name: CF_API_EMAIL
          value: $CLOUDFLARE_EMAIL
EOF
```

#### Deploying a demo service

Now we'll deploy a service to route to on the service mesh.

```bash
cat <<EOF | kubectl apply -f -
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
EOF
```

#### Creating a Gateway instance

And we'll deploy our `Gateway` instance.

```bash
cat <<EOF | kubectl apply -f -
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: example-gateway
  annotations:
    "external-dns.alpha.kubernetes.io/hostname": $DNS_HOSTNAME
spec:
  gatewayClassName: default-consul-gateway-class
  listeners:
  - protocol: HTTPS
    hostname: $DNS_HOSTNAME
    port: 443
    name: https
    allowedRoutes:
      namespaces:
        from: Same
    tls:
      certificateRefs:
        - name: gateway-production-certificate
EOF
```

#### Creating a route to the echo service

Finally, we'll add an `HTTPRoute` to the `Gateway` in order to route
traffic to the service we previously created.

```bash
cat <<EOF | kubectl apply -f -
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: example-route
spec:
  parentRefs:
  - name: example-gateway
  rules:
  - backendRefs:
    - kind: Service
      name: echo
      port: 8080
EOF
```

### Testing the deployment

The cloud load balancer that gets created as part of the `Gateway` deployment
will likely take a few minutes to be provisioned. Once it is assigned an
external IP address, ExternalDNS should create an A record for the IP in your
Cloudflare account.

Once DNS changes have propagated, you can demonstrate that the service on the
mesh can be routed to and that the `Gateway` is using the publically trusted
Let's Encrypt certificate that comes from CertManager.

```bash
curl https://$DNS_HOSTNAME
```

## Cleaning up

Make sure you clean up all of the resources you created, otherwise you will be charged by DigitalOcean.

```bash
export LB_IP=$(kubectl get svc example-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
doctl kubernetes cluster delete demo-cluster
doctl compute load-balancer delete $(doctl compute load-balancer list -o json | jq -r ".[] | select(.ip == \"$LB_IP\") | .id")
```
