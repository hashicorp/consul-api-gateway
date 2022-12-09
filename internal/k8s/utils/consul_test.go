// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestProtocolToConsul(t *testing.T) {
	t.Parallel()

	proto, tls := ProtocolToConsul(gwv1beta1.TCPProtocolType)
	require.Equal(t, proto, "tcp")
	require.False(t, tls)

	proto, tls = ProtocolToConsul(gwv1beta1.TLSProtocolType)
	require.Equal(t, proto, "tcp")
	require.True(t, tls)

	proto, tls = ProtocolToConsul(gwv1beta1.HTTPProtocolType)
	require.Equal(t, proto, "http")
	require.False(t, tls)

	proto, tls = ProtocolToConsul(gwv1beta1.HTTPSProtocolType)
	require.Equal(t, proto, "http")
	require.True(t, tls)

	proto, tls = ProtocolToConsul(gwv1beta1.UDPProtocolType)
	require.Equal(t, proto, "")
	require.False(t, tls)

	proto, tls = ProtocolToConsul(gwv1beta1.ProtocolType("unknown"))
	require.Equal(t, proto, "")
	require.False(t, tls)
}
