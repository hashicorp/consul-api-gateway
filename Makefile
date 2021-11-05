SHELL := /usr/bin/env bash -euo pipefail -c

REPO_NAME    ?= $(shell basename "$(CURDIR)")
PRODUCT_NAME ?= $(REPO_NAME)
BIN_NAME     ?= $(PRODUCT_NAME)

# Get local ARCH; on Intel Mac, 'uname -m' returns x86_64 which we turn into amd64.
# Not using 'go env GOOS/GOARCH' here so 'make docker' will work without local Go install.
ARCH     = $(shell A=$$(uname -m); [ $$A = x86_64 ] && A=amd64; echo $$A)
OS       = $(shell uname | tr [[:upper:]] [[:lower:]])
PLATFORM = $(OS)/$(ARCH)
DIST     = dist/$(PLATFORM)
BIN      = $(DIST)/$(BIN_NAME)

ifneq (,$(wildcard internal/version/version_ent.go))
VERSION = $(shell ./dev/version internal/version/version_ent.go)
else
VERSION = $(shell ./dev/version internal/version/version.go)
endif

# Get latest revision (no dirty check for now).
REVISION = $(shell git rev-parse HEAD)

# Kubernetes-specific stuff
CRD_OPTIONS ?= "crd:trivialVersions=true,allowDangerousTypes=true"

################

# find or download goimports
# download goimports if necessary
.PHONY: goimports
goimports:
ifeq (, $(shell which goimports))
	@go install golang.org/x/tools/cmd/goimports@latest
endif
GOIMPORTS=$(shell which goimports)

.PHONY: fmt
fmt: goimports
	@for d in $$(go list -f {{.Dir}} ./...); do ${GOIMPORTS} --local github.com/hashicorp --local github.com/hashicorp/consul-api-gateway -w -l $$d/*.go; done

.PHONY: test
test:
	go test ./...

# Run controller tests
.PHONY: ctrl-test
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
ctrl-test: ctrl-generate ctrl-manifests
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/master/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./...

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: ctrl-manifests
ctrl-manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=consul-api-gateway-controller webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate code
.PHONY: ctrl-generate
ctrl-generate: controller-gen
	$(CONTROLLER_GEN) object paths="./..."

# find or download controller-gen
# download controller-gen if necessary
.PHONY: controller-gen
controller-gen:
ifeq (, $(shell which controller-gen))
	@go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.0
endif
CONTROLLER_GEN=$(shell which controller-gen)

.PHONY: version
version:
	@echo $(VERSION)

dist:
	mkdir -p $(DIST)
	echo '*' > dist/.gitignore

.PHONY: bin
bin: dist
	GOARCH=$(ARCH) GOOS=$(OS) go build -o $(BIN)

# Docker Stuff.
export DOCKER_BUILDKIT=1
BUILD_ARGS = BIN_NAME=$(BIN_NAME) PRODUCT_VERSION=$(VERSION) PRODUCT_REVISION=$(REVISION)
TAG        = $(PRODUCT_NAME)/$(TARGET):$(VERSION)
BA_FLAGS   = $(addprefix --build-arg=,$(BUILD_ARGS))
FLAGS      = --target $(TARGET) --platform $(PLATFORM) --tag $(TAG) $(BA_FLAGS)

# Set OS to linux for all docker/* targets.
docker/%: OS = linux

# DOCKER_TARGET is a macro that generates the build and run make targets
# for a given Dockerfile target.
# Args: 1) Dockerfile target name (required).
#       2) Build prerequisites (optional).
define DOCKER_TARGET
.PHONY: docker/$(1)
docker/$(1): TARGET=$(1)
docker/$(1): $(2)
	docker build $$(FLAGS) .
	@echo 'Image built; run "docker run --rm $$(TAG)" to try it out.'

.PHONY: docker/$(1)/run
docker/$(1)/run: TARGET=$(1)
docker/$(1)/run: docker/$(1)
	docker run --rm $$(TAG)
endef

# Create docker/<target>[/run] targets.
$(eval $(call DOCKER_TARGET,dev,))
$(eval $(call DOCKER_TARGET,default,bin))
$(eval $(call DOCKER_TARGET,debian,bin))

.PHONY: docker
docker: docker/dev
