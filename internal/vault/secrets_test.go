package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSecret(t *testing.T) {
	// Test empty name
	_, err := ParseSecret("")
	assert.EqualError(t, ErrInvalidSecret, err.Error())

	// Test invalid scheme
	_, err = ParseSecret("invalid://")
	assert.EqualError(t, ErrInvalidSecret, err.Error())

	// Test partial set of serialized values
	secret, err := ParseSecret("vault://example.com?altNames=www.example.com&ttl=12h")
	require.NoError(t, err)
	assert.Equal(t, "www.example.com", secret.AltNames)
	assert.Equal(t, "example.com", secret.CommonName)
	assert.Empty(t, secret.IPSANs)
	assert.Empty(t, secret.OtherSANs)
	assert.Equal(t, "12h", secret.TTL)

	// Test full set of serialized values
	secret, err = ParseSecret("vault://example.com?altNames=www.example.com&ipSans=127.0.0.1&otherSans=helloworld.com&ttl=12h")
	require.NoError(t, err)
	assert.Equal(t, "www.example.com", secret.AltNames)
	assert.Equal(t, "example.com", secret.CommonName)
	assert.Equal(t, "127.0.0.1", secret.IPSANs)
	assert.Equal(t, "helloworld.com", secret.OtherSANs)
	assert.Equal(t, "12h", secret.TTL)

	// Test round trip
	secret2, err := ParseSecret(secret.String())
	require.NoError(t, err)
	assert.Equal(t, secret, secret2)
}

func TestSecret_String(t *testing.T) {
	secret := NewSecret("", "", "", "", "")

	// Test empty
	assert.Equal(t, "vault:", secret.String())

	// Test w/ common name
	secret.CommonName = "example.com"
	assert.Equal(t, "vault://example.com", secret.String())

	// Test with partial set of values
	secret.AltNames = "www.example.com"
	secret.TTL = "12h"
	assert.Equal(t, "vault://example.com?altNames=www.example.com&ttl=12h", secret.String())

	// Test with full set of values
	secret.IPSANs = "127.0.0.1"
	secret.OtherSANs = "helloworld.com"
	assert.Equal(t, "vault://example.com?altNames=www.example.com&ipSans=127.0.0.1&otherSans=helloworld.com&ttl=12h", secret.String())

	// Test round trip
	secret2 := NewSecret("example.com", "www.example.com", "127.0.0.1", "helloworld.com", "12h")
	assert.Equal(t, secret.String(), secret2.String())
}
