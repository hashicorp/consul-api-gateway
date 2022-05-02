package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

var (
	generate bool
)

func init() {
	if os.Getenv("GENERATE") == "true" {
		generate = true
	}
}

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

func TestHTTPRouteDiscoveryChain(t *testing.T) {
	t.Parallel()

	type renderedRoute struct {
		Router    *api.ServiceRouterConfigEntry
		Splitters []*api.ServiceSplitterConfigEntry
	}

	for _, name := range []string{
		"single-service",
		"multiple-services",
		"multiple-rules",
	} {
		t.Run(name, func(t *testing.T) {
			var route core.HTTPRoute

			data, err := os.ReadFile(path.Join("testdata", fmt.Sprintf("%s.json", name)))
			require.NoError(t, err)
			err = json.Unmarshal(data, &route)
			require.NoError(t, err)

			router, splitters := httpRouteDiscoveryChain(route)
			rendered := renderedRoute{
				Router:    router,
				Splitters: splitters,
			}
			data, err = json.MarshalIndent(rendered, "", "  ")
			require.NoError(t, err)

			actual := string(data)

			var expected string
			expectedFileName := fmt.Sprintf("%s.golden.json", name)
			if generate {
				expected = actual
				err := os.WriteFile(path.Join("testdata", expectedFileName), data, 0644)
				require.NoError(t, err)
			} else {
				data, err := os.ReadFile(path.Join("testdata", expectedFileName))
				require.NoError(t, err)
				expected = string(data)
			}

			require.JSONEq(t, expected, actual)
		})
	}
}

func TestConsulSyncAdapter_Sync(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	consulSrv, err := testutil.NewTestServerConfigT(t, func(c *testutil.TestServerConfig) {
		c.Connect = map[string]interface{}{"enabled": true}
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = consulSrv.Stop()
	})

	cfg := api.DefaultConfig()
	cfg.Address = consulSrv.HTTPAddr
	consul, err := api.NewClient(cfg)
	require.NoError(t, err)

	adapter := NewSyncAdapter(testutil.Logger(t), consul)

	route := core.NewTCPRouteBuilder().
		WithName("tcp-default/route1").
		WithService(core.ResolvedService{
			Service: "tcp-default/service1",
		}).
		Build()

	gateway := core.ResolvedGateway{
		ID: core.GatewayID{
			Service: "name1",
		},
		Listeners: []core.ResolvedListener{{
			TLS: core.TLSParams{
				MinVersion: "TLSv1_2",
			},
			Routes: []core.ResolvedRoute{route},
		}},
	}

	_, err = adapter.Sync(ctx, gateway)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		entry, _, err := consul.ConfigEntries().Get(api.IngressGateway, "name1", nil)
		if err != nil {
			return false
		}

		ingress, ok := entry.(*api.IngressGatewayConfigEntry)
		require.True(t, ok)
		require.NotNil(t, ingress)

		return ingress.Listeners[0].TLS.TLSMinVersion == "TLSv1_2" &&
			reflect.DeepEqual(ingress.Listeners[0].TLS.CipherSuites, common.DefaultTLSCipherSuites())
	}, 30*time.Second, 1*time.Second, "listener TLS config not synced in the allotted time")
}
