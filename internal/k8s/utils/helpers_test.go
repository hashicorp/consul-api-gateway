package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamespacedName(t *testing.T) {
	t.Parallel()

	namespacedName := NamespacedName(&core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:      "pod",
			Namespace: "default",
		},
	})
	require.Equal(t, "pod", namespacedName.Name)
	require.Equal(t, "default", namespacedName.Namespace)
}
