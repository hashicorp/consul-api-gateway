package e2e

import (
	"context"
	"os"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
)

const (
	envvarPrefix = "E2E_APIGW_"
)

func SetUpStack(hostRoute string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		var err error
		kindClusterName := envconf.RandomName("consul-api-gateway-test", 30)
		namespace := envconf.RandomName("test", 16)

		ctx = SetHostRoute(ctx, hostRoute)

		for _, f := range []env.Func{
			SetClusterName(kindClusterName),
			SetNamespace(namespace),
			CrossCompileProject,
			BuildDockerImage,
			CreateKindCluster(kindClusterName),
			LoadKindDockerImage(kindClusterName),
			envfuncs.CreateNamespace(namespace),
			InstallGatewayCRDs,
			CreateServiceAccount(namespace),
			CreateTestConsulContainer(kindClusterName, namespace),
			CreateConsulACLPolicy,
			CreateConsulAuthMethod(namespace),
			CreateConsulNamespace,
			InstallConsulAPIGatewayCRDs,
			CreateTestGatewayServer(namespace),
		} {
			ctx, err = f(ctx, cfg)
			if err != nil {
				return nil, err
			}
		}
		return ctx, nil
	}
}

func TearDownStack(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	var err error
	for _, f := range []env.Func{
		DestroyTestGatewayServer,
		envfuncs.DeleteNamespace(Namespace(ctx)),
		DestroyKindCluster(ClusterName(ctx)),
	} {
		ctx, err = f(ctx, cfg)
		if err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

type hostRouteContext struct{}

var (
	hostRouteContextKey = hostRouteContext{}
)

func SetHostRoute(ctx context.Context, hostRoute string) context.Context {
	return context.WithValue(ctx, hostRouteContextKey, hostRoute)
}

func HostRoute(ctx context.Context) string {
	return ctx.Value(hostRouteContextKey).(string)
}

func getEnvDefault(envvar, defaultVal string) string {
	if val := os.Getenv(envvar); val != "" {
		return val
	}
	return defaultVal
}
