module github.com/hashicorp/consul-api-gateway

go 1.16

require (
	github.com/Microsoft/hcsshim v0.9.0 // indirect
	github.com/armon/go-metrics v0.3.9
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/containerd/continuity v0.2.1 // indirect
	github.com/docker/docker v20.10.9+incompatible
	github.com/envoyproxy/go-control-plane v0.9.10-0.20211015211602-cfdef0997689
	github.com/go-logr/logr v0.4.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.2.0
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/hashicorp/consul/api v1.10.1-0.20210924170522-581357c32a29
	github.com/hashicorp/consul/sdk v0.7.0
	github.com/hashicorp/go-hclog v0.16.2
	github.com/hashicorp/go-multierror v1.1.0
	github.com/mitchellh/cli v1.1.2
	github.com/moby/sys/mount v0.2.0 // indirect
	github.com/onsi/gomega v1.15.0 // indirect
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	github.com/vladimirvivien/gexe v0.1.1
	go.uber.org/zap v1.19.0 // indirect
	golang.org/x/net v0.0.0-20210825183410-e898025ed96a
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/grpc v1.40.0
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.21.3
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/klog/v2 v2.10.0
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/e2e-framework v0.0.3
	sigs.k8s.io/gateway-api v0.4.0
)
