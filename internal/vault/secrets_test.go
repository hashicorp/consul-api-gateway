// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePKISecret(t *testing.T) {
	// Test empty name
	_, err := ParsePKISecret("")
	assert.EqualError(t, ErrInvalidSecret, err.Error())

	// Test invalid scheme
	_, err = ParsePKISecret("invalid://")
	assert.EqualError(t, ErrInvalidSecret, err.Error())

	// Test partial set of serialized values
	secret, err := ParsePKISecret("vault+pki://example.com?altNames=www.example.com&ttl=12h")
	require.NoError(t, err)
	assert.Equal(t, "www.example.com", secret.AltNames)
	assert.Equal(t, "example.com", secret.CommonName)
	assert.Empty(t, secret.IPSANs)
	assert.Empty(t, secret.OtherSANs)
	assert.Equal(t, "12h", secret.TTL)

	// Test full set of serialized values
	secret, err = ParsePKISecret("vault+pki://example.com?altNames=www.example.com&ipSans=127.0.0.1&otherSans=helloworld.com&ttl=12h")
	require.NoError(t, err)
	assert.Equal(t, "www.example.com", secret.AltNames)
	assert.Equal(t, "example.com", secret.CommonName)
	assert.Equal(t, "127.0.0.1", secret.IPSANs)
	assert.Equal(t, "helloworld.com", secret.OtherSANs)
	assert.Equal(t, "12h", secret.TTL)

	// Test round trip
	secret2, err := ParsePKISecret(secret.String())
	require.NoError(t, err)
	assert.Equal(t, secret, secret2)
}

func TestPKISecret_String(t *testing.T) {
	secret := NewPKISecret("", "", "", "", "")

	// Test empty
	assert.Equal(t, "vault+pki:", secret.String())

	// Test w/ common name
	secret.CommonName = "example.com"
	assert.Equal(t, "vault+pki://example.com", secret.String())

	// Test with partial set of values
	secret.AltNames = "www.example.com"
	secret.TTL = "12h"
	assert.Equal(t, "vault+pki://example.com?altNames=www.example.com&ttl=12h", secret.String())

	// Test with full set of values
	secret.IPSANs = "127.0.0.1"
	secret.OtherSANs = "helloworld.com"
	assert.Equal(t, "vault+pki://example.com?altNames=www.example.com&ipSans=127.0.0.1&otherSans=helloworld.com&ttl=12h", secret.String())

	// Test round trip
	secret2, err := ParsePKISecret(secret.String())
	require.NoError(t, err)
	assert.Equal(t, secret, secret2)
}

func TestParseKVSecret(t *testing.T) {
	// Test empty name
	_, err := ParseKVSecret("")
	assert.EqualError(t, ErrInvalidSecret, err.Error())

	// Test invalid scheme
	_, err = ParseKVSecret("invalid://")
	assert.EqualError(t, ErrInvalidSecret, err.Error())

	// Test partial set of serialized values
	_, err = ParseKVSecret("vault+kv:///kv/api-gateway-tls-cert?tlsCertField=tls.cert")
	assert.EqualError(t, err, ErrInvalidSecret.Error())
	_, err = ParseKVSecret("vault+kv:///kv/api-gateway-tls-cert?tlsPrivateKeyField=tls.key")
	assert.EqualError(t, err, ErrInvalidSecret.Error())

	// Test full set of serialized values
	secret, err := ParseKVSecret("vault+kv:///kv/api-gateway-tls-cert?tlsCertField=tls.cert&tlsPrivateKeyField=tls.key")
	require.NoError(t, err)
	assert.Equal(t, "/kv/api-gateway-tls-cert", secret.Path)
	assert.Equal(t, "tls.cert", secret.CertField)
	assert.Equal(t, "tls.key", secret.PrivateKeyField)

	// Test round trip
	secret2, err := ParseKVSecret(secret.String())
	require.NoError(t, err)
	assert.Equal(t, secret, secret2)
}

func TestKVSecret_String(t *testing.T) {
	secret := NewKVSecret("", "", "")

	// Test empty
	assert.Equal(t, "vault+kv:", secret.String())

	// Test with only path
	secret.Path = "/kv/api-gateway-tls-cert"
	assert.Equal(t, "vault+kv:///kv/api-gateway-tls-cert", secret.String())

	// Test with path + certificate key
	secret.CertField = "tls.cert"
	assert.Equal(t, "vault+kv:///kv/api-gateway-tls-cert?tlsCertField=tls.cert", secret.String())

	// Test with all values
	secret.PrivateKeyField = "tls.key"
	assert.Equal(t, "vault+kv:///kv/api-gateway-tls-cert?tlsCertField=tls.cert&tlsPrivateKeyField=tls.key", secret.String())

	// Test round trip
	secret2, err := ParseKVSecret(secret.String())
	require.NoError(t, err)
	assert.Equal(t, secret, secret2)
}
