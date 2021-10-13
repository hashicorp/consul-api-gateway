package reconciler

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteID(t *testing.T) {
	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	meta := meta.ObjectMeta{
		Name:      "name",
		Namespace: "namespace",
	}

	require.Equal(t, "http-namespace/name", NewK8sRoute(&gw.HTTPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "udp-namespace/name", NewK8sRoute(&gw.UDPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tcp-namespace/name", NewK8sRoute(&gw.TCPRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "tls-namespace/name", NewK8sRoute(&gw.TLSRoute{
		ObjectMeta: meta,
	}, config).ID())
	require.Equal(t, "", NewK8sRoute(&core.Pod{
		ObjectMeta: meta,
	}, config).ID())
}

func TestRouteCommonRouteSpec(t *testing.T) {
	config := K8sRouteConfig{
		Logger: hclog.NewNullLogger(),
	}

	expected := gw.CommonRouteSpec{
		ParentRefs: []gw.ParentRef{{
			Name: "expected",
		}},
	}

	require.Equal(t, expected, NewK8sRoute(&gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gw.UDPRoute{
		Spec: gw.UDPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gw.TCPRoute{
		Spec: gw.TCPRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, expected, NewK8sRoute(&gw.TLSRoute{
		Spec: gw.TLSRouteSpec{
			CommonRouteSpec: expected,
		},
	}, config).CommonRouteSpec())
	require.Equal(t, gw.CommonRouteSpec{}, NewK8sRoute(&core.Pod{}, config).CommonRouteSpec())
}
