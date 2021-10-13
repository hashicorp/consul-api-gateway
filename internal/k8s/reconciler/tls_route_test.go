package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestTLSRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "tls-namespace/name", TLSRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}
