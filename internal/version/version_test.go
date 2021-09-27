package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	require.Equal(t, "0.1.0-dev", GetHumanVersion())

	GitCommit = "1"
	require.Equal(t, "0.1.0-dev (1)", GetHumanVersion())

	GitDescribe = "description"
	require.Equal(t, "description-dev (1)", GetHumanVersion())

	GitDescribe = ""
	VersionPrerelease = ""
	require.Equal(t, "0.1.0-dev (1)", GetHumanVersion())
}
