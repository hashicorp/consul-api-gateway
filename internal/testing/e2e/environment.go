// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2e

import (
	"context"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type namespaceContext struct{}
type namespaceMirroringContext struct{}
type clusterNameContext struct{}

var namespaceContextKey = namespaceContext{}
var namespaceMirroringContextKey = namespaceMirroringContext{}
var clusterNameContextKey = clusterNameContext{}

func SetNamespace(namespace string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return context.WithValue(ctx, namespaceContextKey, namespace), nil
	}
}

func SetNamespaceMirroring(namespaceMirroring bool) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return context.WithValue(ctx, namespaceMirroringContextKey, namespaceMirroring), nil
	}
}

func Namespace(ctx context.Context) string {
	namespace := ctx.Value(namespaceContextKey)
	if namespace == nil {
		panic("must run this with an integration test that has called SetNamespace")
	}
	return namespace.(string)
}

func NamespaceMirroring(ctx context.Context) bool {
	namespaceMirroring := ctx.Value(namespaceMirroringContextKey)
	return namespaceMirroring.(bool)
}

func SetClusterName(name string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return context.WithValue(ctx, clusterNameContextKey, name), nil
	}
}

func ClusterName(ctx context.Context) string {
	name := ctx.Value(clusterNameContextKey)
	if name == nil {
		panic("must run this with an integration test that has called SetClusterName")
	}
	return name.(string)
}
