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
CRD_OPTIONS ?= "crd:allowDangerousTypes=true"

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

.PHONY: lint
lint:
ifeq (, $(shell which golangci-lint))
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45
endif
	@$(shell which golangci-lint) run --verbose

.PHONY: test
test:
	go test ./...

generate-golden-files:
	GENERATE=true go test ./internal/adapters/consul
	GENERATE=true go test ./internal/envoy
	GENERATE=true go test ./internal/k8s/builder

.PHONY: gen
gen:
ifeq (, $(shell which oapi-codegen))
	@go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen
endif
	go generate ./...

.PHONY: changelog
changelog:
ifeq (, $(shell which changelog-build))
	@go install github.com/hashicorp/go-changelog/cmd/changelog-build@latest
endif
ifeq (, $(LAST_RELEASE_GIT_TAG))
	@echo "Please set the LAST_RELEASE_GIT_TAG environment variable to generate a changelog section of notes since the last release."
else
	@changelog-build -last-release ${LAST_RELEASE_GIT_TAG} -entries-dir .changelog/ -changelog-template .changelog/changelog.tmpl -note-template .changelog/release-note.tmpl -this-release $(shell git rev-parse HEAD)
endif

.PHONY: changelog-entry
changelog-entry:
ifeq (, $(shell which changelog-entry))
	@go install github.com/hashicorp/go-changelog/cmd/changelog-entry@latest
endif
	changelog-entry -dir .changelog

.PHONY: changelog-check
changelog-check:
ifeq (, $(shell which changelog-check))
	@rm -rf go-changelog
	@git clone -b changelog-check https://github.com/mikemorris/go-changelog
	@cd go-changelog && go install ./cmd/changelog-check
endif
	@changelog-check

# Run controller tests
.PHONY: ctrl-test
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
ctrl-test: ctrl-generate ctrl-manifests
ifeq (, $(shell which setup-envtest))
	@go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
endif
	setup-envtest use

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
	@go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.2
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
