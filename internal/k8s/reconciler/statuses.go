// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reconciler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

var _ store.StatusUpdater = (*StatusUpdater)(nil)

type StatusUpdater struct {
	logger         hclog.Logger
	client         gatewayclient.Client
	deployer       *GatewayDeployer
	controllerName string
}

func NewStatusUpdater(logger hclog.Logger, client gatewayclient.Client, deployer *GatewayDeployer, controllerName string) *StatusUpdater {
	return &StatusUpdater{
		logger:         logger,
		client:         client,
		deployer:       deployer,
		controllerName: controllerName,
	}
}

func (s *StatusUpdater) UpdateGatewayStatusOnSync(ctx context.Context, gateway store.Gateway, sync func() (bool, error)) error {
	g := gateway.(*K8sGateway)

	// we've done all but synced our state, so ensure our deployments are up-to-date
	if err := s.deployer.Deploy(ctx, g); err != nil {
		return err
	}

	didSync, err := sync()
	if err != nil {
		g.GatewayState.Status.InSync.SyncError = err
	} else if didSync {
		// clear out any old synchronization error statuses
		g.GatewayState.Status.InSync = status.GatewayInSyncStatus{}
	}

	gatewayStatus := g.GatewayState.GetStatus(g.Gateway)
	if !status.GatewayStatusEqual(gatewayStatus, g.Gateway.Status) {
		g.Gateway.Status = gatewayStatus
		if s.logger.IsTrace() {
			data, err := json.MarshalIndent(gatewayStatus, "", "  ")
			if err == nil {
				s.logger.Trace("setting gateway status", "status", string(data))
			}
		}
		if err := s.client.UpdateStatus(ctx, g.Gateway); err != nil {
			// make sure we return an error immediately that's unwrapped
			return err
		}
	}
	return nil
}

func (s *StatusUpdater) UpdateRouteStatus(ctx context.Context, route store.Route) error {
	r := route.(*K8sRoute)

	if status, ok := r.RouteState.ParentStatuses.NeedsUpdate(r.routeStatus(), s.controllerName, r.GetGeneration()); ok {
		r.setStatus(status)

		if s.logger.IsTrace() {
			status, err := json.MarshalIndent(status, "", "  ")
			if err == nil {
				s.logger.Trace("syncing route status", "status", string(status))
			}
		}
		if err := s.client.UpdateStatus(ctx, r.Route); err != nil {
			return fmt.Errorf("error updating route status: %w", err)
		}
	}

	return nil
}
