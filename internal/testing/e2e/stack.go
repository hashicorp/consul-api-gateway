package e2e

import (
	"context"
	"os"
	"path/filepath"

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
		namespaces := []string{
			envconf.RandomName("test-ns1", 16),
			envconf.RandomName("test-ns2", 16),
		}

		ctx = SetHostRoute(ctx, hostRoute)

		for _, f := range []env.Func{
			SetClusterName(kindClusterName),
			SetNamespaces(namespaces),
			CrossCompileProject,
			BuildDockerImage,
			CreateKindCluster(kindClusterName),
			LoadKindDockerImage(kindClusterName),
			// TODO: is there a cleaner way to iterate over namespaces?
			envfuncs.CreateNamespace(namespaces[0]),
			envfuncs.CreateNamespace(namespaces[1]),
			InstallGatewayCRDs,
			CreateServiceAccount(namespaces[0], "consul-api-gateway", getBasePath()+"/config/rbac/role.yaml"),
			CreateTestConsulContainer(kindClusterName, namespaces[0]),
			CreateConsulACLPolicy,
			CreateConsulAuthMethod(),
			CreateConsulNamespace,
			InstallConsulAPIGatewayCRDs,
			CreateTestGatewayServer(namespaces[0]),
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
	namespaces := Namespaces(ctx)

	for _, f := range []env.Func{
		DestroyTestGatewayServer,
		// TODO: is there a cleaner way to iterate over namespaces?
		envfuncs.DeleteNamespace(namespaces[0]),
		envfuncs.DeleteNamespace(namespaces[1]),
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

func getBasePath() string {
	path, _ := filepath.Abs("../../.././")
	return path
}
