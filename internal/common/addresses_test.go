// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressTypeForAddress(t *testing.T) {
	require.Equal(t, "STATIC", AddressTypeForAddress("127.0.0.1"))
	require.Equal(t, "STATIC", AddressTypeForAddress("::"))
	require.Equal(t, "STRICT_DNS", AddressTypeForAddress("test.com"))
}
