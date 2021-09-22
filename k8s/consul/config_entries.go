package consul

import (
	"github.com/hashicorp/consul/api"
)

type ConfigEntryIndex struct {
	kind string
	idx  map[string]api.ConfigEntry
}

func NewConfigEntryIndex(kind string) *ConfigEntryIndex {
	return &ConfigEntryIndex{
		kind: kind,
		idx:  map[string]api.ConfigEntry{},
	}
}

func (i *ConfigEntryIndex) Add(entry api.ConfigEntry) {
	if entry.GetKind() != i.kind {
		return
	}
	i.idx[entry.GetName()] = entry
}

func (i *ConfigEntryIndex) Merge(other *ConfigEntryIndex) {
	if i.kind != other.kind {
		return
	}
	for k, v := range other.idx {
		i.idx[k] = v
	}
}

func (i *ConfigEntryIndex) Get(name string) (api.ConfigEntry, bool) {
	c, ok := i.idx[name]
	return c, ok
}

func (i *ConfigEntryIndex) Count() int {
	return len(i.idx)
}

// Difference will return an ConfigEntryIndex with entries that not found in the current ConfigEntryIndex
func (i *ConfigEntryIndex) Difference(other *ConfigEntryIndex) *ConfigEntryIndex {
	return i.filter(other, false)
}

func (i *ConfigEntryIndex) Intersection(other *ConfigEntryIndex) *ConfigEntryIndex {
	return i.filter(other, true)
}

func (i *ConfigEntryIndex) filter(other *ConfigEntryIndex, include bool) *ConfigEntryIndex {
	result := NewConfigEntryIndex(i.kind)
	for _, c := range other.idx {
		if _, ok := i.idx[c.GetName()]; ok && include {
			// we're looking for the set that is in i
			result.Add(c)
		} else if !ok && !include {
			// we're looking for the set that isn't in i
			result.Add(c)
		}
	}
	return result
}

func (i *ConfigEntryIndex) ToArray() []api.ConfigEntry {
	result := make([]api.ConfigEntry, len(i.idx))
	var j int
	for _, c := range i.idx {
		result[j] = c
		j++
	}
	return result
}
