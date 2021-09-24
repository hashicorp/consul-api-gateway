package server

import (
	"bytes"
	"context"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	require.Equal(t, "", testCmd().Help())
	require.Equal(t, "Starts the consul-api-gateway control plane server", testCmd().Synopsis())

	ctx := context.Background()

	// flag checking
	var buffer bytes.Buffer

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-not-a-flag",
	}))
	require.Contains(t, buffer.String(), "flag provided but not defined: -not-a-flag")
	buffer.Reset()
}

func testCmd() *Command {
	ui := cli.NewMockUi()
	return &Command{UI: ui}
}
