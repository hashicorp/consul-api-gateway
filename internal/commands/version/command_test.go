// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	ui := cli.NewMockUi()
	cmd := &Command{
		UI:      ui,
		Version: "1",
	}
	require.NotEmpty(t, cmd.Help())
	require.Equal(t, "Prints the version", cmd.Synopsis())

	require.Equal(t, 0, cmd.Run(nil))
	require.Equal(t, "consul-api-gateway 1\n", ui.OutputWriter.String())
}
