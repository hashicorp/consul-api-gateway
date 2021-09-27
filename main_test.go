package main

import (
	"bytes"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestMain(t *testing.T) {
	ui := cli.NewMockUi()
	var buffer bytes.Buffer

	require.Equal(t, 0, run([]string{
		"server", "-h",
	}, ui, &buffer))
	require.NotEmpty(t, buffer.String())
	buffer.Reset()

	require.Equal(t, 0, run([]string{
		"exec", "-h",
	}, ui, &buffer))
	require.NotEmpty(t, buffer.String())
	buffer.Reset()

	require.Equal(t, 0, run([]string{
		"version", "-h",
	}, ui, &buffer))
	require.NotEmpty(t, buffer.String())
	buffer.Reset()

	require.Equal(t, 0, run([]string{
		"-h",
	}, ui, &buffer))
	require.NotEmpty(t, buffer.String())
	buffer.Reset()
}

func TestHelpFilter(t *testing.T) {
	ui := cli.NewMockUi()
	var buffer bytes.Buffer

	commands := initializeCommands(ui, &buffer)
	output := helpFunc(commands)(commands)

	require.NotContains(t, output, "exec")
}
