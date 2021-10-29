package e2e

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"

	"github.com/cenkalti/backoff"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type k8sTokenContext struct{}

var k8sTokenContextKey = k8sTokenContext{}

const gatewayCRDs = "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.0"

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
	)
	meta.AddToGroupVersion(scheme.Scheme, gateway.SchemeGroupVersion)

	return ctx, nil
}

func CreateServiceAccount(namespace string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Print("Creating service account")

		if err := cfg.Client().Resources().Create(ctx, serviceAccount(namespace)); err != nil {
			return nil, err
		}
		if err := cfg.Client().Resources().Create(ctx, serviceClusterRole(namespace)); err != nil {
			return nil, err
		}
		if err := cfg.Client().Resources().Create(ctx, serviceClusterRoleTokenBinding(namespace)); err != nil {
			return nil, err
		}
		if err := cfg.Client().Resources().Create(ctx, serviceClusterRoleAuthBinding(namespace)); err != nil {
			return nil, err
		}

		var secretName string
		err := backoff.Retry(func() error {
			account := &core.ServiceAccount{}
			if err := cfg.Client().Resources().Get(ctx, "consul-api-gateway", namespace, account); err != nil {
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

func serviceAccount(namespace string) *core.ServiceAccount {
	return &core.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-api-gateway",
			Namespace: namespace,
		},
	}
}

func serviceClusterRole(namespace string) *rbac.ClusterRole {
	return &rbac.ClusterRole{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-api-gateway-auth",
			Namespace: namespace,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"serviceaccounts"},
				Verbs:     []string{"get"},
			},
		},
	}
}

func serviceClusterRoleTokenBinding(namespace string) *rbac.ClusterRoleBinding {
	return &rbac.ClusterRoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-api-gateway-tokenreview-binding",
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
				Name:      "consul-api-gateway",
				Namespace: namespace,
			},
		},
	}
}

func serviceClusterRoleAuthBinding(namespace string) *rbac.ClusterRoleBinding {
	return &rbac.ClusterRoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name:      "consul-api-gateway-auth-binding",
			Namespace: namespace,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "consul-api-gateway-auth",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "consul-api-gateway",
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
