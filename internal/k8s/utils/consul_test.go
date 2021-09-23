package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestProtocolToConsul(t *testing.T) {
	t.Parallel()

	proto, tls := ProtocolToConsul(gateway.TCPProtocolType)
	require.Equal(t, proto, "tcp")
	require.False(t, tls)

	proto, tls = ProtocolToConsul(gateway.TLSProtocolType)
	require.Equal(t, proto, "tcp")
	require.True(t, tls)

	proto, tls = ProtocolToConsul(gateway.HTTPProtocolType)
	require.Equal(t, proto, "http")
	require.False(t, tls)

	proto, tls = ProtocolToConsul(gateway.HTTPSProtocolType)
	require.Equal(t, proto, "http")
	require.True(t, tls)

	proto, tls = ProtocolToConsul(gateway.UDPProtocolType)
	require.Equal(t, proto, "")
	require.False(t, tls)

	proto, tls = ProtocolToConsul(gateway.ProtocolType("unknown"))
	require.Equal(t, proto, "")
	require.False(t, tls)
}
