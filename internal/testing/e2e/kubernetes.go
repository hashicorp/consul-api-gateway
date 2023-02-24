// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"github.com/cenkalti/backoff"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	consulapigw "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

type (
	k8sConsulTokenContext        struct{}
	k8sConsulGatewayTokenContext struct{}
)

var (
	k8sConsulTokenContextKey        = k8sConsulTokenContext{}
	k8sConsulGatewayTokenContextKey = k8sConsulGatewayTokenContext{}
)

func InstallCRDs(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Print("Installing CRDs")

	dir := path.Join("..", "..", "..", "config", "crd")
	crds, err := kubectlKustomizeCRDs(ctx, dir)
	if err != nil {
		return nil, err
	}

	if _, err := envtest.InstallCRDs(cfg.Client().RESTConfig(), envtest.CRDInstallOptions{
		CRDs: crds,
	}); err != nil {
		return nil, err
	}

	// Register Gateway API types
	scheme.Scheme.AddKnownTypes(
		gwv1beta1.SchemeGroupVersion,
		&gwv1beta1.GatewayClass{},
		&gwv1beta1.GatewayClassList{},
		&gwv1beta1.Gateway{},
		&gwv1beta1.GatewayList{},
	)
	meta.AddToGroupVersion(scheme.Scheme, gwv1beta1.SchemeGroupVersion)

	scheme.Scheme.AddKnownTypes(
		gwv1alpha2.SchemeGroupVersion,
		&gwv1alpha2.HTTPRoute{},
		&gwv1alpha2.HTTPRouteList{},
		&gwv1alpha2.TCPRoute{},
		&gwv1alpha2.TCPRouteList{},
		&gwv1alpha2.ReferenceGrant{},
		&gwv1alpha2.ReferencePolicy{},
	)
	meta.AddToGroupVersion(scheme.Scheme, gwv1alpha2.SchemeGroupVersion)

	// Register Consul API Gateway types
	consulapigw.RegisterTypes(scheme.Scheme)

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

		// As of K8s 1.24, ServiceAccounts no longer implicitly create a Secret w/ token,
		// so we create the token Secret directly using the proper annotations.
		// https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/#to-create-additional-api-tokens
		if err := cfg.Client().Resources().Create(ctx, &core.Secret{
			Type: core.SecretTypeServiceAccountToken,
			ObjectMeta: meta.ObjectMeta{
				Name:        accountName,
				Namespace:   namespace,
				Annotations: map[string]string{core.ServiceAccountNameKey: accountName},
			},
		}); err != nil {
			return nil, errors.New("failed to create secret w/ service account token")
		}

		var token string
		err = backoff.Retry(func() error {
			secret := &core.Secret{}
			err = cfg.Client().Resources().Get(ctx, accountName, namespace, secret)
			if err != nil {
				return err
			}

			token = string(secret.Data[core.ServiceAccountTokenKey])
			if token == "" {
				return errors.New("service account token not added to Secret")
			}

			return nil
		}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 5), ctx))
		if err != nil {
			return nil, err
		}
		if accountName == "consul-server" {
			return context.WithValue(ctx, k8sConsulTokenContextKey, token), nil
		} else {
			return context.WithValue(ctx, k8sConsulGatewayTokenContextKey, token), nil
		}
	}
}

func K8sConsulServiceToken(ctx context.Context) string {
	token := ctx.Value(k8sConsulTokenContextKey)
	log.Printf("consul token: %s\n", token)
	if token == nil {
		panic("must run this with an integration test that has called CreateServiceAccount")
	}
	return token.(string)
}

func K8sConsulGatewayServiceToken(ctx context.Context) string {
	token := ctx.Value(k8sConsulGatewayTokenContextKey)
	log.Printf("consul gateway token: %s\n", token)
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

func readCRDs(data []byte) ([]*api.CustomResourceDefinition, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), len(data))
	crds := []*api.CustomResourceDefinition{}
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
	config := rest.CopyConfig(client.RESTConfig())
	config.BearerToken = K8sConsulGatewayServiceToken(ctx)

	// overwrite the TLS config so we're not using cert-based auth
	tlsConfig := client.RESTConfig().TLSClientConfig
	config.TLSClientConfig = rest.TLSClientConfig{
		ServerName: tlsConfig.ServerName,
		CAFile:     tlsConfig.CAFile,
		CAData:     tlsConfig.CAData,
	}
	return klient.New(config)
}
