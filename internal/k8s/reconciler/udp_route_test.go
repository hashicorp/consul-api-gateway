package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestUDPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "udp-namespace/name", UDPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}
