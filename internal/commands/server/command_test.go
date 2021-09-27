package server

import (
	"context"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
)

func TestServerHelpSynopsis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ui := cli.NewMockUi()
	var buffer gwTesting.Buffer
	cmd := New(ctx, ui, &buffer)
	cmd.isTest = true

	require.Equal(t, "Starts the consul-api-gateway control plane server", cmd.Synopsis())
	require.NotEmpty(t, cmd.Help())
}

func TestExec(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		args   []string
		retVal int
		output string
	}{{
		name: "flag-parse-error",
		args: []string{
			"-not-a-flag",
		},
		retVal: 1,
		output: "flag provided but not defined: -not-a-flag",
	}, {
		name: "invalid-context",
		args: []string{
			"-ca-file", "file",
			"-ca-secret-namespace", "namespace",
			"-ca-secret", "secret",
			"-consul-address", "localhost",
			"-k8s-context", "thiscontextdoesnotexist",
		},
		retVal: 1,
		output: "error getting kubernetes configuration",
	}} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			ui := cli.NewMockUi()
			var buffer gwTesting.Buffer
			cmd := New(ctx, ui, &buffer)
			cmd.isTest = true

			require.Equal(t, test.retVal, cmd.Run(test.args))
			require.Contains(t, buffer.String(), test.output)
		})
	}
}
