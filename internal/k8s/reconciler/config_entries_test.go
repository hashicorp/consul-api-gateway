package reconciler

import (
	"fmt"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
)

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
