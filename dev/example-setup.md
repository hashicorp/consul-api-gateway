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
doctl kubernetes cluster create gateway-controller-cluster --node-pool "name=worker-pool;size=s-2vcpu-2gb;count=1"
```

## Set up gateway crds

```bash
kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.0" | kubectl apply -f -
```

## Set up Consul

```bash
cat <<EOF | helm install consul hashicorp/consul --version 0.33.0 -f -
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
doctl kubernetes cluster registry add gateway-controller-cluster
doctl registry login gateway
doctl registry kubernetes-manifest | kubectl apply -f -
kubectl patch serviceaccount default -p '{"imagePullSecrets": [{"name": "registry-gateway"}]}'
GOOS=linux go build
docker build -t registry.digitalocean.com/gateway/controller:1 .
docker push registry.digitalocean.com/gateway/controller:1
```

## (PRE PUBLIC RELEASE ONLY) Set up installation kustomizations

```bash
mkdir -p tmp
cat <<EOF > tmp/kustomization.yaml 
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../config

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
kubectl apply -f config/crd/bases/api-gateway.consul.hashicorp.com_gatewayclassconfigs.yaml
# For PRE PUBLIC RELEASE ONLY
kubectl apply -k tmp
# kubectl apply -k config
```

## Install cert manager through helm

```bash
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --version v1.5.3 --set installCRDs=true
```

## Install external dns CRDs

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/external-dns/65a69275b1f76fa01b56a708d0514ae49edf30fd/docs/contributing/crd-source/crd-manifest.yaml
```

## Set up example deployment kustomizations

```bash
cat <<EOF > tmp/kustomization.yaml 
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../config/example

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
```

## Create gateway resources

```bash
kubectl apply -k tmp
```

## Create external DNS entry

When the load balancer service has an external ip, then run.

```bash
export LB_IP=$(kubectl get svc example-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
cat <<EOF | kubectl apply -f -
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: gateway
spec:
  endpoints:
  - dnsName: $DNS_HOSTNAME
    recordTTL: 180
    recordType: A
    targets:
    - $LB_IP
EOF
```

## Test

Once DNS has propagated

```bash
curl https://$DNS_HOSTNAME
```

## Clean up resources

```bash
kubectl delete dnsendpoint gateway
doctl registry delete gateway
doctl kubernetes cluster delete gateway-controller-cluster
```