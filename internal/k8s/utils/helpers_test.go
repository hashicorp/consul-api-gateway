package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestNamespacedName(t *testing.T) {
	t.Parallel()

	namespacedName := NamespacedName(&core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:      "pod",
			Namespace: "default",
		},
	})
	require.Equal(t, "pod", namespacedName.Name)
	require.Equal(t, "default", namespacedName.Namespace)
}

func TestIsFieldUpdated(t *testing.T) {
	t.Parallel()

	input := map[string]string{
		"test": "value",
	}
	updated := map[string]string{
		"test":   "value",
		"second": "value",
	}
	require.False(t, IsFieldUpdated(input, input))
	require.True(t, IsFieldUpdated(input, updated))
}

func TestHostnamesForHTTPRoute(t *testing.T) {
	t.Parallel()

	hostnames := HostnamesForHTTPRoute("", &gateway.HTTPRoute{})
	require.Len(t, hostnames, 0)

	hostnames = HostnamesForHTTPRoute("default.host.name", &gateway.HTTPRoute{})
	require.Len(t, hostnames, 1)
	require.Equal(t, "default.host.name", hostnames[0])

	hostnames = HostnamesForHTTPRoute("default.host.name", &gateway.HTTPRoute{
		Spec: gateway.HTTPRouteSpec{
			Hostnames: []gateway.Hostname{"1.host.name"},
		},
	})
	require.Len(t, hostnames, 1)
	require.Equal(t, "1.host.name", hostnames[0])

	hostnames = HostnamesForHTTPRoute("default.host.name", &gateway.HTTPRoute{
		Spec: gateway.HTTPRouteSpec{
			Hostnames: []gateway.Hostname{"1.host.name", "2.host.name"},
		},
	})
	require.Len(t, hostnames, 2)
	require.Equal(t, "1.host.name", hostnames[0])
	require.Equal(t, "2.host.name", hostnames[1])
}
