package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsManagedGateway(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		labels   map[string]string
		expected bool
		gateway  string
	}{{
		name: "unmanaged",
		labels: map[string]string{
			nameLabel: "unmanaged",
		},
		expected: false,
	}, {
		name: "unnamed",
		labels: map[string]string{
			ManagedLabel: "true",
		},
		expected: false,
	}, {
		name: "valid",
		labels: map[string]string{
			ManagedLabel: "true",
			nameLabel:    "test-gateway",
		},
		expected: true,
		gateway:  "test-gateway",
	}} {
		t.Run(test.name, func(t *testing.T) {
			actualGateway, actual := IsManagedGateway(test.labels)
			require.Equal(t, test.expected, actual)
			require.Equal(t, test.gateway, actualGateway)
		})
	}
}
