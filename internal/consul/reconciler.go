package consul

import (
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
		if _, _, err := c.consul.ConfigEntries().Set(entry, nil); err != nil {
			c.logger.Error("error setting entry", "kind", entry.GetKind(), "name", entry.GetName(), "error", err)
		}
	}
}

func (c *ConfigEntriesReconciler) DeleteConfigEntries(entries ...api.ConfigEntry) {
	for _, entry := range entries {
		// TODO: handle failures?
		c.logger.Debug("deleting entry", "kind", entry.GetKind(), "name", entry.GetName())
		if _, err := c.consul.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), nil); err != nil {
			c.logger.Error("error deleting entry", "kind", entry.GetKind(), "name", entry.GetName(), "error", err)
		}
	}
}

func (c *ConfigEntriesReconciler) Reconcile(igw api.ConfigEntry, computedRouters, computedSplitters, computedDefaults *ConfigEntryIndex) error {
	// Since we can't make multiple config entry changes in a single transaction we must
	// perform the operations in a set that is least likely to induce downtime.
	// First the new service-defaults, routers and splitters should be set
	// Second the ingress gateway
	// Third the removal of any service-defaults, routers or splitters that no longer exist
	// TODO: what happens if we get an error here? we could leak config entries if we get an error on removal, maybe they should get garbage collected by consul-api-gateway?

	addedRouters := computedRouters.ToArray()
	addedDefaults := computedDefaults.ToArray()
	addedSplitters := computedSplitters.ToArray()
	removedRouters := computedRouters.Difference(c.routers).ToArray()
	removedSplitters := computedSplitters.Difference(c.splitters).ToArray()
	removedDefaults := computedDefaults.Difference(c.defaults).ToArray()

	if c.logger.IsTrace() {
		c.logger.Trace("adding config entries", "routers", entryNames(addedRouters), "splitters", entryNames(addedSplitters), "defaults", entryNames(addedDefaults))
		c.logger.Trace("removing config entries", "routers", entryNames(removedRouters), "splitters", entryNames(removedSplitters), "defaults", entryNames(removedDefaults))
	}

	// defaults need to go first, otherwise the routers are always configured to use tcp
	c.SetConfigEntries(addedDefaults...)
	c.SetConfigEntries(addedRouters...)
	c.SetConfigEntries(addedSplitters...)

	c.SetConfigEntries(igw)

	c.DeleteConfigEntries(removedRouters...)
	c.DeleteConfigEntries(removedSplitters...)
	c.DeleteConfigEntries(removedDefaults...)

	c.routers = computedRouters
	c.splitters = computedSplitters
	c.defaults = computedDefaults

	return nil
}

func entryNames(entries []api.ConfigEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.GetName()
	}
	return names
}
