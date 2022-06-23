package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/cenkalti/backoff"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type k8sTokenContext struct{}

var k8sTokenContextKey = k8sTokenContext{}

const gatewayCRDs = "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.1"

func InstallGatewayCRDs(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Installing Gateway CRDs")

	crds, err := kubectlKustomizeCRDs(ctx, gatewayCRDs)
	if err != nil {
		return nil, err
	}

	if _, err := envtest.InstallCRDs(cfg.Client().RESTConfig(), envtest.CRDInstallOptions{
		CRDs: crds,
	}); err != nil {
		return nil, err
	}

	scheme.Scheme.AddKnownTypes(
		gateway.SchemeGroupVersion,
		&gateway.GatewayClass{},
		&gateway.GatewayClassList{},
		&gateway.Gateway{},
		&gateway.GatewayList{},
		&gateway.HTTPRoute{},
		&gateway.HTTPRouteList{},
		&gateway.TCPRoute{},
		&gateway.TCPRouteList{},
		&gateway.ReferencePolicy{},
	)
	meta.AddToGroupVersion(scheme.Scheme, gateway.SchemeGroupVersion)

	return ctx, nil
}

func CreateServiceAccount(namespace, accountName, clusterRolePath string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Print("Creating service account")

		if err := cfg.Client().Resources().Create(ctx, serviceAccount(namespace, accountName)); err != nil {
			return nil, err
		}
		clusterRole, err := serviceClusterRole(namespace, accountName, clusterRolePath)
		if err != nil {
			return nil, err
		}
		if err := cfg.Client().Resources().Create(ctx, clusterRole); err != nil {
			return nil, err
		}
		// Used for Consul auth-method login in deployments
		if err := cfg.Client().Resources().Create(ctx, serviceClusterRoleTokenBinding(namespace, accountName)); err != nil {
			return nil, err
		}
		if err := cfg.Client().Resources().Create(ctx, serviceClusterRoleAuthBinding(namespace, accountName)); err != nil {
			return nil, err
		}

		var secretName string
		err = backoff.Retry(func() error {
			account := &core.ServiceAccount{}
			if err := cfg.Client().Resources().Get(ctx, accountName, namespace, account); err != nil {
				return err
			}
			if len(account.Secrets) == 0 {
				return errors.New("invalid account secrets")
			}
			secretName = account.Secrets[0].Name
			return nil
		}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 5), ctx))
		if err != nil {
			return nil, err
		}

		secret := &core.Secret{}
		if err := cfg.Client().Resources().Get(ctx, secretName, namespace, secret); err != nil {
			return nil, err
		}

		return context.WithValue(ctx, k8sTokenContextKey, string(secret.Data["token"])), nil
	}
}

func K8sServiceToken(ctx context.Context) string {
	token := ctx.Value(k8sTokenContextKey)
	if token == nil {
		panic("must run this with an integration test that has called CreateServiceAccount")
	}
	return token.(string)
}

func serviceAccount(namespace string, accountName string) *core.ServiceAccount {
	return &core.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Name:      accountName,
			Namespace: namespace,
		},
	}
}

func loadConfigFromFilePath(filePath string, object interface{}, objectIsValid func() bool) error {
	//load from path
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(fileBytes), len(fileBytes))

	for !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		err = decoder.Decode(object)
		if objectIsValid() {
			return nil
		}
	}

	return fmt.Errorf("unable to load valid k8s object from filepath: %s", filePath)
}

func serviceClusterRole(namespace string, accountName, clusterRolePath string) (*rbac.ClusterRole, error) {
	if clusterRolePath != "" {
		clusterRole := &rbac.ClusterRole{}

		err := loadConfigFromFilePath(clusterRolePath, clusterRole, func() bool {
			return len(clusterRole.Rules) > 0
		})
		if err != nil {
			return nil, err
		}
		if len(clusterRole.Rules) == 0 {
			return nil, errors.New("unable to load valid clusterrole from " + clusterRolePath)
		}

		//update name and namespaces to match the test
		clusterRole.Name = accountName + "-auth"
		clusterRole.Namespace = namespace
		return clusterRole, nil

	}

	return &rbac.ClusterRole{
		ObjectMeta: meta.ObjectMeta{
			Name:      accountName + "-auth",
			Namespace: namespace,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"serviceaccounts"},
				Verbs:     []string{"get"},
			},
		},
	}, nil
}

func serviceClusterRoleTokenBinding(namespace string, accountName string) *rbac.ClusterRoleBinding {
	return &rbac.ClusterRoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name:      accountName + "-tokenreview-binding",
			Namespace: namespace,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:auth-delegator",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      accountName,
				Namespace: namespace,
			},
		},
	}
}

func serviceClusterRoleAuthBinding(namespace, accountName string) *rbac.ClusterRoleBinding {
	return &rbac.ClusterRoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name:      accountName + "-auth-binding",
			Namespace: namespace,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     accountName + "-auth",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      accountName,
				Namespace: namespace,
			},
		},
	}
}

func readCRDs(data []byte) ([]client.Object, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), len(data))
	crds := []client.Object{}
	for {
		crd := &api.CustomResourceDefinition{}
		err := decoder.Decode(crd)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if crd.Name != "" {
			crds = append(crds, crd)
		}
	}
	return crds, nil
}

func serviceAccountClient(ctx context.Context, client klient.Client, account, namespace string) (klient.Client, error) {
	serviceAccount := core.ServiceAccount{}
	if err := client.Resources().Get(ctx, account, namespace, &serviceAccount); err != nil {
		return nil, err
	}
	if len(serviceAccount.Secrets) == 0 {
		return nil, errors.New("can't find secret")
	}
	secretName := serviceAccount.Secrets[0].Name
	token := core.Secret{}
	if err := client.Resources().Get(ctx, secretName, namespace, &token); err != nil {
		return nil, err
	}
	tokenData, found := token.Data["token"]
	if !found {
		return nil, errors.New("token not found")
	}

	config := rest.CopyConfig(client.RESTConfig())
	tlsConfig := client.RESTConfig().TLSClientConfig

	config.BearerToken = string(tokenData)
	// overwrite the TLS config so we're not using cert-based auth
	config.TLSClientConfig = rest.TLSClientConfig{
		ServerName: tlsConfig.ServerName,
		CAFile:     tlsConfig.CAFile,
		CAData:     tlsConfig.CAData,
	}
	return klient.New(config)
}
