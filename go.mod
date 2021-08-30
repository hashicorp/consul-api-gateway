module github.com/hashicorp/polar

go 1.16

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.12.0 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.1.2
	github.com/hashicorp/consul/api v1.9.1
	github.com/hashicorp/go-hclog v0.16.2
	github.com/mitchellh/cli v1.1.2
	github.com/prometheus/common v0.26.0
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.22.0
	k8s.io/apimachinery v0.22.0
	k8s.io/client-go v0.22.0
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/gateway-api v0.3.1-0.20210817221314-e11957e9082b
)
