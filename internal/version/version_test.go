package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, GetHumanVersion())

	GitCommit = "1"
	require.Contains(t, GetHumanVersion(), "(1)")

	GitDescribe = "description"
	require.Contains(t, GetHumanVersion(), "description")
}
