package consul

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
)

func TestConfigEntryIndexMerge(t *testing.T) {
	mainEntries := []*api.ServiceRouterConfigEntry{{
		Kind: api.ServiceRouter,
		Name: "zero",
	}}
	commonEntries := []*api.ServiceRouterConfigEntry{{
		Kind: api.ServiceRouter,
		Name: "one",
	}, {
		Kind: api.ServiceRouter,
		Name: "two",
	}}
	extraEntries := []*api.ServiceRouterConfigEntry{{
		Kind: api.ServiceRouter,
		Name: "three",
	}, {
		Kind: api.ServiceRouter,
		Name: "four",
	}}

	main := NewConfigEntryIndex(api.ServiceRouter)
	other := NewConfigEntryIndex(api.ServiceRouter)

	for _, entry := range mainEntries {
		main.Add(entry)
	}
	for _, entry := range commonEntries {
		main.Add(entry)
		other.Add(entry)
	}
	for _, entry := range extraEntries {
		other.Add(entry)
	}
	different := NewConfigEntryIndex(api.IngressGateway)
	different.Add(&api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
	})
	different.Add(&api.IngressGatewayConfigEntry{})

	require.Equal(t, 3, main.Count())
	main.Merge(different)
	require.Equal(t, 3, main.Count())
	main.Merge(other)
	require.Equal(t, 5, main.Count())
}

func TestConfigEntryIndexDifference(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		entries    []*api.ServiceRouterConfigEntry
		other      []*api.ServiceRouterConfigEntry
		difference []*api.ServiceRouterConfigEntry
	}{{
		name: "empty",
		other: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}},
		difference: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}},
	}, {
		name: "empty-other",
		entries: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}},
	}, {
		name: "overlapping",
		entries: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}, {
			Kind: api.ServiceRouter,
			Name: "three",
		}},
		other: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "two",
		}, {
			Kind: api.ServiceRouter,
			Name: "three",
		}, {
			Kind: api.ServiceRouter,
			Name: "four",
		}},
		difference: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "four",
		}},
	}} {
		t.Run(test.name, func(t *testing.T) {
			index := NewConfigEntryIndex(api.ServiceRouter)
			other := NewConfigEntryIndex(api.ServiceRouter)
			for _, entry := range test.entries {
				index.Add(entry)
			}
			for _, entry := range test.other {
				other.Add(entry)
			}
			difference := index.Difference(other)
			require.Equal(t, len(test.difference), difference.Count())
			for _, entry := range test.difference {
				_, found := difference.Get(entry.GetName())
				require.True(t, found, fmt.Sprintf("did not find entry '%s', but should have", entry.GetName()))
			}
		})
	}
}

func TestConfigEntryIndexIntersection(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		entries      []*api.ServiceRouterConfigEntry
		other        []*api.ServiceRouterConfigEntry
		intersection []*api.ServiceRouterConfigEntry
	}{{
		name: "empty",
		other: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}},
	}, {
		name: "empty-other",
		entries: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}},
	}, {
		name: "overlapping",
		entries: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "one",
		}, {
			Kind: api.ServiceRouter,
			Name: "two",
		}, {
			Kind: api.ServiceRouter,
			Name: "three",
		}},
		other: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "two",
		}, {
			Kind: api.ServiceRouter,
			Name: "three",
		}, {
			Kind: api.ServiceRouter,
			Name: "four",
		}},
		intersection: []*api.ServiceRouterConfigEntry{{
			Kind: api.ServiceRouter,
			Name: "two",
		}, {
			Kind: api.ServiceRouter,
			Name: "three",
		}},
	}} {
		t.Run(test.name, func(t *testing.T) {
			index := NewConfigEntryIndex(api.ServiceRouter)
			other := NewConfigEntryIndex(api.ServiceRouter)
			for _, entry := range test.entries {
				index.Add(entry)
			}
			for _, entry := range test.other {
				other.Add(entry)
			}
			intersection := index.Intersection(other)
			require.Equal(t, len(test.intersection), intersection.Count())
			for _, entry := range test.intersection {
				_, found := intersection.Get(entry.GetName())
				require.True(t, found, fmt.Sprintf("did not find entry '%s', but should have", entry.GetName()))
			}
		})
	}
}

func TestConfigEntryIndexToArray(t *testing.T) {
	entryOne := &api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
		Name: "one",
	}
	entryTwo := &api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
		Name: "two",
	}
	index := NewConfigEntryIndex(api.IngressGateway)
	index.Add(entryOne)
	index.Add(entryTwo)
	entries := index.ToArray()
	require.Len(t, entries, 2)
	require.ElementsMatch(t, entries, []api.ConfigEntry{entryOne, entryTwo})
}
