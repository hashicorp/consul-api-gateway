package main

import (
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestMain(t *testing.T) {
	ui := cli.NewMockUi()

	require.Equal(t, 0, run([]string{
		"server", "-h",
	}, ui))
	ui.OutputWriter.Reset()

	require.Equal(t, 0, run([]string{
		"exec", "-h",
	}, ui))
	ui.OutputWriter.Reset()

	require.Equal(t, 0, run([]string{
		"version",
	}, ui))
	require.NotEmpty(t, ui.OutputWriter.String())
	ui.OutputWriter.Reset()

	require.Equal(t, 0, run([]string{
		"-h",
	}, ui))
}
