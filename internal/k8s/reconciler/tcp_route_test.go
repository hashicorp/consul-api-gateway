package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestTCPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "tcp-namespace/name", TCPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}
