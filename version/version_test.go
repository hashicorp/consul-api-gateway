package version

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVersion(t *testing.T) {
	version := GetHumanVersion()
	req := require.New(t)
	req.NotEmpty(version, "Version cannot be empty")

}
