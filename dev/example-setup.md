# Digitalocean + Cloudflare + Cert Manager + External DNS Setup

These are basic copy-paste commands for setting up a demo Kubernetes cluster that leverages
this project.

## Set environment variables

```bash
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_EMAIL=...
export DNS_HOSTNAME=...
export DIGITALOCEAN_TOKEN=...
```

## Set up kubernetes cluster

```bash
doctl auth init -t $DIGITALOCEAN_TOKEN
doctl kubernetes cluster create demo-cluster --node-pool "name=worker-pool;size=s-2vcpu-2gb;count=1"
```

## Set up Consul

```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
cat <<EOF | helm install consul hashicorp/consul --version 0.35.0 -f -
global:
  name: consul
  image: "hashicorpdev/consul:581357c32"
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

## (PRE PUBLIC RELEASE ONLY) Set up private docker registry and image

```bash
doctl registry create gateway
doctl kubernetes cluster registry add demo-cluster
doctl registry login gateway
doctl registry kubernetes-manifest | kubectl apply -f -
kubectl patch serviceaccount default -p '{"imagePullSecrets": [{"name": "registry-gateway"}]}'
GOOS=linux go build
docker build -t registry.digitalocean.com/gateway/controller:1 .
docker push registry.digitalocean.com/gateway/controller:1
```

## (PRE PUBLIC RELEASE ONLY) Set up installation kustomizations

```bash
mkdir -p demo-deployment/install
cat <<EOF > demo-deployment/install/kustomization.yaml 
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../../config

images:
- name: hashicorp/consul-api-gateway
  newName: registry.digitalocean.com/gateway/controller
  newTag: "1"

patchesStrategicMerge:
- |-
  apiVersion: api-gateway.consul.hashicorp.com/v1alpha1
  kind: GatewayClassConfig
  metadata:
    name: default-consul-gateway-class-config
  spec:
    image:
      consulAPIGateway: "registry.digitalocean.com/gateway/controller:1"
- |-
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: consul-api-gateway-controller
  spec:
    template:
      spec:
        imagePullSecrets:
          - name: registry-gateway
EOF
```

## Set up gateway controller

```bash
kubectl kustomize config/crd --reorder=none | kubectl apply -f -
# For PRE PUBLIC RELEASE ONLY
kubectl kustomize demo-deployment/install --reorder=none | kubectl apply -f -
# kubectl apply -k config --reorder=none | kubectl apply -f
```

## Install third-party dependencies

```bash
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --version v1.5.3 --set installCRDs=true
```

## Set up example deployment kustomizations

```bash
mkdir -p demo-deployment/example
cat <<EOF > demo-deployment/example/kustomization.yaml 
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../../config/example

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

kubectl kustomize demo-deployment/example --reorder=none | kubectl apply -f -
```

## Test

Once DNS has propagated

```bash
curl https://$DNS_HOSTNAME
```

## Clean up resources

```bash
export LB_IP=$(kubectl get svc example-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
doctl kubernetes cluster delete demo-cluster
doctl registry delete gateway
doctl compute load-balancer delete $(doctl compute load-balancer list -o json | jq -r ".[] | select(.ip == \"$LB_IP\") | .id")
```