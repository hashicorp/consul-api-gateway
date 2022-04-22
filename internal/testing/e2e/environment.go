package e2e

import (
	"context"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type namespaceContext struct{}
type clusterNameContext struct{}

var namespaceContextKey = namespaceContext{}
var clusterNameContextKey = clusterNameContext{}

func SetNamespaces(namespaces []string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return context.WithValue(ctx, namespaceContextKey, namespaces), nil
	}
}

func Namespaces(ctx context.Context) []string {
	namespaces := ctx.Value(namespaceContextKey)
	if namespaces == nil {
		panic("must run this with an integration test that has called SetNamespaces")
	}
	return namespaces.([]string)
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
