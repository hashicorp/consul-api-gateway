package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestHTTPRouteID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http-namespace/name", HTTPRouteID(types.NamespacedName{Namespace: "namespace", Name: "name"}))
}
