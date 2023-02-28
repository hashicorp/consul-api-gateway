// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build tools

// following https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package internal

import (
	_ "github.com/deepmap/oapi-codegen/cmd/oapi-codegen"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/hashicorp/go-changelog/cmd/changelog-build"
	_ "github.com/hashicorp/go-changelog/cmd/changelog-check"
	_ "github.com/hashicorp/go-changelog/cmd/changelog-entry"
	_ "golang.org/x/tools/cmd/goimports"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
