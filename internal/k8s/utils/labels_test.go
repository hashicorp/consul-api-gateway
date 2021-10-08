package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
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

func TestLabelsForGateway(t *testing.T) {
	t.Parallel()

	labels := LabelsForGateway(&gateway.Gateway{
		ObjectMeta: meta.ObjectMeta{
			Name:      "gateway",
			Namespace: "default",
		},
	})
	require.Equal(t, map[string]string{
		nameLabel:      "gateway",
		namespaceLabel: "default",
		ManagedLabel:   "true",
		createdAtLabel: "-62135596800",
	}, labels)
}
