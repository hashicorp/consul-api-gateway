package reconciler

import (
	"context"
	"encoding/json"
	"fmt"

	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

// GatewayDeployer creates gateway deployments and services and ensures that they exist

func (g *K8sGateway) ensureDeploymentExists(ctx context.Context) error {
	// Create service account for the gateway
	if serviceAccount := g.config.ServiceAccountFor(g.Gateway); serviceAccount != nil {
		if err := g.client.EnsureServiceAccount(ctx, g.Gateway, serviceAccount); err != nil {
			return err
		}
	}

	deployment := g.deploymentBuilder.Build()
	mutated := deployment.DeepCopy()
	if updated, err := g.client.CreateOrUpdateDeployment(ctx, mutated, func() error {
		mutated = apigwv1alpha1.MergeDeployment(deployment, mutated)
		return g.client.SetControllerOwnership(g.Gateway, mutated)
	}); err != nil {
		return fmt.Errorf("failed to create or update gateway deployment: %w", err)
	} else if updated {
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(mutated, "", "  ")
			if err == nil {
				g.logger.Trace("created or updated gateway deployment", "deployment", string(data))
			}
		}
	}

	// Create service for the gateway
	if service := g.serviceBuilder.Build(); service != nil {
		mutated := service.DeepCopy()
		if updated, err := g.client.CreateOrUpdateService(ctx, mutated, func() error {
			mutated = apigwv1alpha1.MergeService(service, mutated)
			return g.client.SetControllerOwnership(g.Gateway, mutated)
		}); err != nil {
			return fmt.Errorf("failed to create or update gateway service: %w", err)
		} else if updated {
			if g.logger.IsTrace() {
				data, err := json.MarshalIndent(mutated, "", "  ")
				if err == nil {
					g.logger.Trace("created or updated gateway service", "service", string(data))
				}
			}
		}
	}

	return nil
}
