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

func SetNamespace(namespace string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return context.WithValue(ctx, namespaceContextKey, namespace), nil
	}
}

func Namespace(ctx context.Context) string {
	namespace := ctx.Value(namespaceContextKey)
	if namespace == nil {
		panic("must run this with an integration test that has called SetNamespace")
	}
	return namespace.(string)
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
