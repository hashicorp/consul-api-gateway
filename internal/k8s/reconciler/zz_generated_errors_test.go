package reconciler

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCertificateResolutionErrorType(t *testing.T) {
	t.Parallel()

	expected := "expected"

	require.Equal(t, expected, NewCertificateResolutionErrorNotFound(expected).Error())
	require.Equal(t, CertificateResolutionErrorTypeNotFound, NewCertificateResolutionErrorNotFound(expected).Kind())
	require.Equal(t, expected, NewCertificateResolutionErrorUnsupported(expected).Error())
	require.Equal(t, CertificateResolutionErrorTypeUnsupported, NewCertificateResolutionErrorUnsupported(expected).Kind())
}

func TestBindErrorType(t *testing.T) {
	t.Parallel()

	expected := "expected"

	require.Equal(t, expected, NewBindErrorRouteKind(expected).Error())
	require.Equal(t, BindErrorTypeRouteKind, NewBindErrorRouteKind(expected).Kind())
	require.Equal(t, expected, NewBindErrorListenerNamespacePolicy(expected).Error())
	require.Equal(t, BindErrorTypeListenerNamespacePolicy, NewBindErrorListenerNamespacePolicy(expected).Kind())
	require.Equal(t, expected, NewBindErrorHostnameMismatch(expected).Error())
	require.Equal(t, BindErrorTypeHostnameMismatch, NewBindErrorHostnameMismatch(expected).Kind())
}
