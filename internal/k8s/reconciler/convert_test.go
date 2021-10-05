package reconciler

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul/api"
)

func TestHTTPRouteToServiceDiscoChain(t *testing.T) {
	// helper to quickly define k8s http backend refs. Each ref can be given in the format of <name>:<weight> where weight is optional
	mkBackends := func(backends ...string) []gw.HTTPBackendRef {
		refs := make([]gw.HTTPBackendRef, len(backends))
		for i, backend := range backends {
			parts := strings.Split(backend, ":")
			ref := gw.BackendRef{
				BackendObjectReference: gw.BackendObjectReference{Name: parts[0]},
			}
			if len(parts) == 2 {
				if weight, err := strconv.Atoi(parts[1]); err == nil {
					w := int32(weight)
					ref.Weight = &w
				}
			}
			refs[i] = gw.HTTPBackendRef{BackendRef: ref}
		}
		return refs
	}

	mkServiceRtr := func(name string, meta map[string]string, routes ...api.ServiceRoute) *api.ServiceRouterConfigEntry {
		return &api.ServiceRouterConfigEntry{
			Kind:   api.ServiceRouter,
			Name:   name,
			Routes: routes,
			Meta:   meta,
		}
	}

	cases := []struct {
		name     string
		route    *gw.HTTPRoute
		prefix   string
		meta     map[string]string
		router   *api.ServiceRouterConfigEntry
		splits   []*api.ServiceSplitterConfigEntry
		backends []string
	}{
		{
			name: "Simple no split",
			route: &gw.HTTPRoute{
				ObjectMeta: meta.ObjectMeta{
					Name: "test",
				},
				Spec: gw.HTTPRouteSpec{
					Rules: []gw.HTTPRouteRule{
						{
							BackendRefs: mkBackends("backend"),
						},
					},
				},
			},
			router: mkServiceRtr("test", nil, api.ServiceRoute{Destination: &api.ServiceRouteDestination{Service: "backend"}}),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require := require.New(t)

			router, splits := HTTPRouteToServiceDiscoChain(c.route, c.prefix, c.meta)
			require.Equal(c.router, router)
			require.EqualValues(c.splits, splits)
		})
	}
}
