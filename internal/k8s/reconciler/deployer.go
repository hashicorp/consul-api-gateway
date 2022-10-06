package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/consul-api-gateway/internal/consul"
	capi "github.com/hashicorp/consul/api"

	"github.com/hashicorp/go-hclog"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/builder"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

// GatewayDeployer creates gateway deployments and services and ensures that they exist
type GatewayDeployer struct {
	client                    gatewayclient.Client
	consulCA                  string
	primaryDatacenter         string
	sdsHost                   string
	sdsPort                   int
	consul                    *capi.Client
	consulNamespaceMirrioring bool

	logger hclog.Logger
}

type DeployerConfig struct {
	ConsulCA                  string
	PrimaryDatacenter         string
	SDSHost                   string
	SDSPort                   int
	Logger                    hclog.Logger
	Client                    gatewayclient.Client
	Consul                    *capi.Client
	ConsulNamespaceMirrioring bool
}

func NewDeployer(config DeployerConfig) *GatewayDeployer {
	return &GatewayDeployer{
		client:                    config.Client,
		consulCA:                  config.ConsulCA,
		primaryDatacenter:         config.PrimaryDatacenter,
		sdsHost:                   config.SDSHost,
		sdsPort:                   config.SDSPort,
		logger:                    config.Logger,
		consul:                    config.Consul,
		consulNamespaceMirrioring: config.ConsulNamespaceMirrioring,
	}
}

func (d *GatewayDeployer) Deploy(ctx context.Context, gateway *K8sGateway) error {
	if d.consulNamespaceMirrioring {
		_, err := consul.EnsureNamespaceExists(d.consul, gateway.Namespace, "")
		if err != nil {
			return err
		}
	}

	if err := d.ensureServiceAccount(ctx, gateway.config, gateway.Gateway); err != nil {
		return err
	}

	if err := d.ensureSecret(ctx, gateway.config, gateway.Gateway); err != nil {
		return err
	}

	if err := d.ensureDeployment(ctx, gateway.GatewayState.ConsulNamespace, gateway.config, gateway.Gateway); err != nil {
		return err
	}

	return d.ensureService(ctx, gateway.config, gateway.Gateway)
}

func (d *GatewayDeployer) ensureServiceAccount(ctx context.Context, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	// Create service account for the gateway
	serviceAccount := config.ServiceAccountFor(gateway)
	if serviceAccount == nil {
		return nil
	}

	return d.client.EnsureServiceAccount(ctx, gateway, serviceAccount)
}

// ensureSecret makes sure there is a Secret in the same namespace as the Gateway
// containing the Consul CA certificate for the Gateway pod(s) to mount as a volume.
func (d *GatewayDeployer) ensureSecret(ctx context.Context, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	// Only deploy the Secret if the config requires CA
	if config.Spec.ConsulSpec.Scheme != "https" {
		return nil
	}

	secret := &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name:      gateway.Name,
			Namespace: gateway.Namespace,
			Labels:    utils.LabelsForGateway(gateway),
		},
		Data: map[string][]byte{
			"consul-ca-cert": []byte(d.consulCA),
		},
	}

	mutated := secret.DeepCopy()

	updated, err := d.client.CreateOrUpdateSecret(ctx, mutated, func() error {
		mutated = apigwv1alpha1.MergeSecret(secret, mutated)
		return d.client.SetControllerOwnership(gateway, mutated)
	})
	if err != nil {
		return fmt.Errorf("failed to create or update gateway secret: %w", err)
	}

	if updated && d.logger.IsTrace() {
		d.logger.Trace("created or updated gateway secret")
	}

	return nil
}

func (d *GatewayDeployer) ensureDeployment(ctx context.Context, namespace string, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	// get current deployment so user set replica count isn't overridden by default values
	currentDeployment, err := d.client.GetDeployment(ctx, types.NamespacedName{Namespace: gateway.Namespace, Name: gateway.Name})
	if err != nil {
		return err
	}
	var currentReplicas *int32
	if currentDeployment != nil {
		currentReplicas = currentDeployment.Spec.Replicas
	}

	deployment := d.Deployment(namespace, config, gateway, currentReplicas)
	mutated := deployment.DeepCopy()

	updated, err := d.client.CreateOrUpdateDeployment(ctx, mutated, func() error {
		mutated = apigwv1alpha1.MergeDeployment(deployment, mutated)
		return d.client.SetControllerOwnership(gateway, mutated)
	})
	if err != nil {
		return fmt.Errorf("failed to create or update gateway deployment: %w", err)
	}

	if updated && d.logger.IsTrace() {
		data, err := json.MarshalIndent(mutated, "", "  ")
		if err == nil {
			d.logger.Trace("created or updated gateway deployment", "deployment", string(data))
		}
	}

	return nil
}

func (d *GatewayDeployer) ensureService(ctx context.Context, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	service := d.Service(config, gateway)
	if service == nil {
		return nil
	}

	mutated := service.DeepCopy()
	updated, err := d.client.CreateOrUpdateService(ctx, mutated, func() error {
		mutated = apigwv1alpha1.MergeService(service, mutated)
		return d.client.SetControllerOwnership(gateway, mutated)
	})
	if err != nil {
		return fmt.Errorf("failed to create or update gateway service: %w", err)
	}

	if updated && d.logger.IsTrace() {
		data, err := json.MarshalIndent(mutated, "", "  ")
		if err == nil {
			d.logger.Trace("created or updated gateway service", "service", string(data))
		}
	}

	return nil
}

func (d *GatewayDeployer) Deployment(namespace string, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway, currentReplicas *int32) *apps.Deployment {
	return builder.NewGatewayDeployment(gateway).
		WithSDS(d.sdsHost, d.sdsPort).
		WithClassConfig(config).
		WithConsulCA(d.consulCA).
		WithConsulGatewayNamespace(namespace).
		WithPrimaryConsulDatacenter(d.primaryDatacenter).
		Build(currentReplicas)
}

func (d *GatewayDeployer) Service(config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) *core.Service {
	return builder.NewGatewayService(gateway).
		WithClassConfig(config).
		Build()
}
