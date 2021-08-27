https://gateway-api.sigs.k8s.io/v1alpha2/guides/simple-gateway/

./scripts/develop

overmind s

./scripts/bootstrap

cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: prod-web
spec:
  gatewayClassName: acme-lb
  listeners:  
  - protocol: HTTP
    port: 80
    name: prod-web-gw
    allowedRoutes:
      namespaces:
        from: Same
EOF