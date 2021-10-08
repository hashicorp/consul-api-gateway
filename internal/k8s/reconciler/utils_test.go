package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostnamesMatch(t *testing.T) {
	require.True(t, hostnamesMatch("*", "*"))
	require.True(t, hostnamesMatch("", "*"))
	require.True(t, hostnamesMatch("*", ""))
	require.True(t, hostnamesMatch("", ""))
	require.True(t, hostnamesMatch("*.test", "*.test"))
	require.True(t, hostnamesMatch("a.test", "*.test"))
	require.True(t, hostnamesMatch("*.test", "a.test"))
	require.False(t, hostnamesMatch("*.test", "a.b.test"))
	require.False(t, hostnamesMatch("a.b.test", "*.test"))
	require.True(t, hostnamesMatch("a.b.test", "*.b.test"))
	require.True(t, hostnamesMatch("*.b.test", "a.b.test"))
	require.False(t, hostnamesMatch("*.b.test", "a.c.test"))
	require.True(t, hostnamesMatch("a.b.test", "a.b.test"))
}
