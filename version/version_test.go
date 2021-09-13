package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	version := GetHumanVersion()
	req := require.New(t)
	req.NotEmpty(version, "Version cannot be empty")

}
