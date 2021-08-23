package consul

import (
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type ConfigEntriesReconciler struct {
	consul *api.Client
	logger hclog.Logger

	routers   *ConfigEntryIndex
	splitters *ConfigEntryIndex
	defaults  *ConfigEntryIndex
}

func NewReconciler(c *api.Client, logger hclog.Logger) *ConfigEntriesReconciler {
	return &ConfigEntriesReconciler{
		consul:    c,
		logger:    logger.Named("consul"),
		routers:   NewConfigEntryIndex(api.ServiceRouter),
		splitters: NewConfigEntryIndex(api.ServiceSplitter),
		defaults:  NewConfigEntryIndex(api.ServiceDefaults),
	}
}

func (c *ConfigEntriesReconciler) SetConfigEntries(entries ...api.ConfigEntry) {
	for _, entry := range entries {
		// TODO: handle failures?
		c.logger.Debug("setting entry", "kind", entry.GetKind(), "name", entry.GetName())
		c.consul.ConfigEntries().Set(entry, nil)
	}
}

func (c *ConfigEntriesReconciler) DeleteConfigEntries(entries ...api.ConfigEntry) {
	for _, entry := range entries {
		// TODO: handle failures?
		c.logger.Debug("deleting entry", "kind", entry.GetKind(), "name", entry.GetName())
		c.consul.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), nil)
	}
}

func (c *ConfigEntriesReconciler) ReconcileGateway(gw *ResolvedGateway) error {
	igw, computedRouters, computedSplitters, computedDefaults, err := gw.computeConfigEntries()
	if err != nil {
		return fmt.Errorf("failed to reconcile config entries: %w", err)
	}

	// Since we can't make multiple config entry changes in a single transaction we must
	// perform the operations in a set that is least likely to induce downtime.
	// First the new service-defaults, routers and splitters should be set
	// Second the ingress gateway
	// Third the removal of any service-defaults, routers or splitters that no longer exist
	// TODO: what happens if we get an error here? we could leak config entries if we get an error on removal, maybe they should get garbage collected by polar?

	c.SetConfigEntries(computedRouters.ToArray()...)
	c.SetConfigEntries(computedSplitters.ToArray()...)
	c.SetConfigEntries(computedDefaults.ToArray()...)

	c.SetConfigEntries(igw)

	c.DeleteConfigEntries(computedRouters.Difference(c.routers).ToArray()...)
	c.DeleteConfigEntries(computedSplitters.Difference(c.splitters).ToArray()...)
	c.DeleteConfigEntries(computedDefaults.Difference(c.defaults).ToArray()...)

	c.routers = computedRouters
	c.splitters = computedSplitters
	c.defaults = computedDefaults

	return nil
}
