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

	// require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
	// 	"-ca-file", "/notafile",
	// }))
	// require.Contains(t, buffer.String(), "Error loading CA File")
	// buffer.Reset()
	//
	// require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
	// 	"-ca-secret-namespace", "default",
	// 	"-ca-secret", "/notafile",
	// }))
	// require.Contains(t, buffer.String(), "unable to pull Consul CA cert from secret")
	// buffer.Reset()
	//
	// timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	// defer cancel()
	// require.Equal(t, 1, testCmd().run(timeoutCtx, &buffer, []string{
	// 	"-consul-address", "notadomain",
	// }))
	// require.Contains(t, buffer.String(), "timeout waiting for certs to be written")
	// buffer.Reset()
}

func testCmd() *Command {
	ui := cli.NewMockUi()
	return &Command{UI: ui}
}
