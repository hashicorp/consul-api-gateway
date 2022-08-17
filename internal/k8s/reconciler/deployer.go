package reconciler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-hclog"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/builder"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

// GatewayDeployer creates gateway deployments and services and ensures that they exist
type GatewayDeployer struct {
	client   gatewayclient.Client
	consulCA string
	sdsHost  string
	sdsPort  int

	logger hclog.Logger
}

type DeployerConfig struct {
	ConsulCA string
	SDSHost  string
	SDSPort  int
	Logger   hclog.Logger
	Client   gatewayclient.Client
}

func NewDeployer(config DeployerConfig) *GatewayDeployer {
	return &GatewayDeployer{
		client:   config.Client,
		consulCA: config.ConsulCA,
		sdsHost:  config.SDSHost,
		sdsPort:  config.SDSPort,
		logger:   config.Logger,
	}
}

func (d *GatewayDeployer) Deploy(ctx context.Context, namespace string, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	if err := d.ensureServiceAccount(ctx, config, gateway); err != nil {
		return err
	}

	if err := d.ensureDeployment(ctx, namespace, config, gateway); err != nil {
		return err
	}

	return d.ensureService(ctx, config, gateway)
}

func (d *GatewayDeployer) ensureServiceAccount(ctx context.Context, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	// Create service account for the gateway
	if serviceAccount := config.ServiceAccountFor(gateway); serviceAccount != nil {
		if err := d.client.EnsureServiceAccount(ctx, gateway, serviceAccount); err != nil {
			return err
		}
	}
	return nil
}

func (d *GatewayDeployer) ensureDeployment(ctx context.Context, namespace string, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) error {
	deployment := d.Deployment(namespace, config, gateway)
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

func (d *GatewayDeployer) Deployment(namespace string, config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) *apps.Deployment {
	deploymentBuilder := builder.NewGatewayDeployment(gateway)
	deploymentBuilder.WithSDS(d.sdsHost, d.sdsPort)
	deploymentBuilder.WithClassConfig(config)
	deploymentBuilder.WithConsulCA(d.consulCA)
	deploymentBuilder.WithConsulGatewayNamespace(namespace)
	return deploymentBuilder.Build(nil) // TODO?
}

func (d *GatewayDeployer) Service(config apigwv1alpha1.GatewayClassConfig, gateway *gwv1beta1.Gateway) *core.Service {
	serviceBuilder := builder.NewGatewayService(gateway)
	serviceBuilder.WithClassConfig(config)
	return serviceBuilder.Build()
}
