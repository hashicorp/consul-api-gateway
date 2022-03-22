package consul

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

// TestRouteConsolidator verifies that various combinations of hostnames and rules
// are consolidated into a list with one route per hostname and all rules for hostname.
func TestRouteConsolidator(t *testing.T) {
	c := newRouteConsolidator()

	g := core.ResolvedGateway{
		ID:   core.GatewayID{},
		Meta: map[string]string{`name`: t.Name()},
		Listeners: []core.ResolvedListener{
			{Name: t.Name()},
		},
	}

	basePathMatch := core.HTTPMatch{Path: core.HTTPPathMatch{Type: core.HTTPPathMatchPrefixType, Value: "/"}}
	v1HeaderMatch := core.HTTPMatch{Headers: []core.HTTPHeaderMatch{{Name: "version", Value: "one"}}}
	v2PathMatch := core.HTTPMatch{Path: core.HTTPPathMatch{Type: core.HTTPPathMatchPrefixType, Value: "/v2"}}
	v2HeaderMatch := core.HTTPMatch{Headers: []core.HTTPHeaderMatch{{Name: "version", Value: "two"}}}

	route1 := core.HTTPRoute{
		Hostnames: []string{`example.com`, `example.net`},
		Rules: []core.HTTPRouteRule{
			{
				Matches: []core.HTTPMatch{basePathMatch, v1HeaderMatch},
			},
		},
	}

	route2 := core.HTTPRoute{
		Hostnames: []string{`example.com`},
		Rules: []core.HTTPRouteRule{
			{
				Matches: []core.HTTPMatch{v2PathMatch, v2HeaderMatch},
			},
		},
	}

	c.add(route1)
	c.add(route2)
	routes := c.consolidate(g)

	// We should have 2 routes, each w/ one hostname
	require.Len(t, routes, 2)
	require.Len(t, routes[0].Hostnames, 1)
	require.Len(t, routes[1].Hostnames, 1)

	comRoute, netRoute := routes[0], routes[1]
	if comRoute.Hostnames[0] != "example.com" {
		netRoute, comRoute = routes[0], routes[1]
	}

	// example.net has a subset of example.com's matches
	assert.Equal(t, "example.net", netRoute.Hostnames[0])
	require.Len(t, netRoute.Rules, 2)
	assert.Equal(t, []core.HTTPMatch{basePathMatch}, comRoute.Rules[1].Matches)
	assert.Equal(t, []core.HTTPMatch{v1HeaderMatch}, comRoute.Rules[2].Matches)

	// example.com has a couple of extra matches
	assert.Equal(t, "example.com", comRoute.Hostnames[0])
	require.Len(t, comRoute.Rules, 4)
	assert.Equal(t, []core.HTTPMatch{v2PathMatch}, comRoute.Rules[0].Matches)
	assert.Equal(t, []core.HTTPMatch{basePathMatch}, comRoute.Rules[1].Matches)
	assert.Equal(t, []core.HTTPMatch{v1HeaderMatch}, comRoute.Rules[2].Matches)
	assert.Equal(t, []core.HTTPMatch{v2HeaderMatch}, comRoute.Rules[3].Matches)
}
