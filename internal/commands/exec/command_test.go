package exec

import (
	"bytes"
	"context"
	"os"
	"path"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func init() {
	isTest = true
}

func TestExec(t *testing.T) {
	require.Equal(t, "consul-api-gateway exec command", testCmd().Synopsis())
	require.NotEmpty(t, testCmd().Help())

	ctx := context.Background()

	// flag checking
	var buffer bytes.Buffer

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-not-a-flag",
	}))
	require.Contains(t, buffer.String(), "flag provided but not defined: -not-a-flag")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, nil))
	require.Contains(t, buffer.String(), "-consul-http-address must be set")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "localhost",
	}))
	require.Contains(t, buffer.String(), "-gateway-host must be set")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "localhost",
		"-gateway-host", "localhost",
	}))
	require.Contains(t, buffer.String(), "-gateway-name must be set")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "localhost",
		"-gateway-host", "localhost",
		"-gateway-name", "gateway",
	}))
	require.Contains(t, buffer.String(), "-envoy-bootstrap-path must be set")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "localhost",
		"-gateway-host", "localhost",
		"-gateway-name", "gateway",
		"-envoy-bootstrap-path", "/path.json",
	}))
	require.Contains(t, buffer.String(), "-envoy-sds-address must be set")
	buffer.Reset()

	// error handling

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "localhost",
		"-gateway-host", "localhost",
		"-gateway-name", "gateway",
		"-envoy-bootstrap-path", "/path.json",
		"-envoy-sds-address", "localhost",
		"-consul-ca-cert-file", "/not-a-file",
	}))
	require.Contains(t, buffer.String(), "error creating consul client")
	require.Contains(t, buffer.String(), "/not-a-file: no such file or directory")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "notadomain",
		"-gateway-host", "localhost",
		"-gateway-name", "gateway",
		"-envoy-bootstrap-path", "/path.json",
		"-envoy-sds-address", "localhost",
		"-acl-auth-method", "no-auth-method",
		"-acl-bearer-token-file", "/notafile",
	}))
	require.Contains(t, buffer.String(), "error reading bearer token")
	buffer.Reset()

	directory, err := os.MkdirTemp("", "exec")
	require.NoError(t, err)
	defer os.RemoveAll(directory)
	file := path.Join(directory, "token")
	err = os.WriteFile(file, []byte("token"), 0600)
	require.NoError(t, err)

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "notadomain",
		"-gateway-host", "localhost",
		"-gateway-name", "gateway",
		"-envoy-bootstrap-path", "/path.json",
		"-envoy-sds-address", "localhost",
		"-acl-auth-method", "no-auth-method",
		"-acl-bearer-token-file", file,
	}))
	require.Contains(t, buffer.String(), "error logging into consul")
	buffer.Reset()

	require.Equal(t, 1, testCmd().run(ctx, &buffer, []string{
		"-consul-http-address", "notadomain",
		"-gateway-host", "localhost",
		"-gateway-name", "gateway",
		"-envoy-bootstrap-path", "/path.json",
		"-envoy-sds-address", "localhost",
	}))
	require.Contains(t, buffer.String(), "error registering service")
	buffer.Reset()
}

func testCmd() *Command {
	ui := cli.NewMockUi()
	return &Command{UI: ui}
}
