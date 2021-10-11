package consul

import (
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
)

func TestFiltersToModifier(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		filters  []core.HTTPFilter
		expected *api.HTTPHeaderModifiers
	}{{
		name: "basic",
		filters: []core.HTTPFilter{{
			Type: core.HTTPRedirectFilterType,
		}, {
			Type: core.HTTPHeaderFilterType,
			Header: core.HTTPHeaderFilter{
				Add: map[string]string{
					"a": "b",
				},
				Set: map[string]string{
					"c": "d",
				},
				Remove: []string{"e"},
			},
		}},
		expected: &api.HTTPHeaderModifiers{
			Add: map[string]string{
				"a": "b",
			},
			Set: map[string]string{
				"c": "d",
			},
			Remove: []string{"e"},
		},
	}, {
		name: "merge",
		filters: []core.HTTPFilter{{
			Type: core.HTTPHeaderFilterType,
			Header: core.HTTPHeaderFilter{
				Add: map[string]string{
					"a": "b",
				},
				Set: map[string]string{
					"c": "d",
				},
				Remove: []string{"e"},
			},
		}, {
			Type: core.HTTPHeaderFilterType,
			Header: core.HTTPHeaderFilter{
				Add: map[string]string{
					"a": "d",
				},
				Set: map[string]string{
					"c": "d",
				},
				Remove: []string{"f"},
			},
		}},
		expected: &api.HTTPHeaderModifiers{
			Add: map[string]string{
				"a": "d",
			},
			Set: map[string]string{
				"c": "d",
			},
			Remove: []string{"e", "f"},
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			actual := httpRouteFiltersToServiceRouteHeaderModifier(test.filters)
			require.EqualValues(t, test.expected, actual)
		})
	}
}
