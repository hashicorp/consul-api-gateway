package main

import (
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestHelpFilter(t *testing.T) {
	ui := cli.NewMockUi()

	commands := initializeCommands(ui)
	output := helpFunc(commands)(commands)

	require.NotContains(t, output, "exec")
}
