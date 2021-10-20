module github.com/hashicorp/consul-api-gateway

go 1.16

require (
	github.com/armon/go-metrics v0.3.9
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/envoyproxy/go-control-plane v0.9.10-0.20211015211602-cfdef0997689
	github.com/go-logr/logr v0.4.0
	github.com/golang/mock v1.5.0
	github.com/google/uuid v1.1.2
	github.com/hashicorp/consul/api v1.10.1-0.20210924170522-581357c32a29
	github.com/hashicorp/go-hclog v0.16.2
	github.com/hashicorp/go-multierror v1.1.0
	github.com/mitchellh/cli v1.1.2
	github.com/onsi/gomega v1.15.0 // indirect
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.0 // indirect
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/grpc v1.40.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/klog/v2 v2.10.0
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/gateway-api v0.4.0
)
