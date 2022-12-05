// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"context"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"

	gwTesting "github.com/hashicorp/consul-api-gateway/internal/testing"
)

func TestExecHelpSynopsis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ui := cli.NewMockUi()
	var buffer gwTesting.Buffer
	cmd := New(ctx, ui, &buffer)
	cmd.isTest = true

	require.Equal(t, "consul-api-gateway exec command", cmd.Synopsis())
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
		name:   "consul-http-address-required",
		retVal: 1,
		output: "-consul-http-address must be set",
	}, {
		name:   "gateway-host-required",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
		},
		output: "-gateway-host must be set",
	}, {
		name:   "gateway-name-required",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
			"-gateway-host", "localhost",
		},
		output: "-gateway-name must be set",
	}, {
		name:   "envoy-bootstrap-path-required",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
			"-gateway-host", "localhost",
			"-gateway-name", "gateway",
		},
		output: "-envoy-bootstrap-path must be set",
	}, {
		name:   "envoy-sds-address-required",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
			"-gateway-host", "localhost",
			"-gateway-name", "gateway",
			"-envoy-bootstrap-path", "/path.json",
		},
		output: "-envoy-sds-address must be set",
	}, {
		name:   "envoy-sds-address-required",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
			"-gateway-host", "localhost",
			"-gateway-name", "gateway",
			"-envoy-bootstrap-path", "/path.json",
		},
		output: "-envoy-sds-address must be set",
	}, {
		name:   "consul-ca-cert-file-error",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
			"-gateway-host", "localhost",
			"-gateway-name", "gateway",
			"-envoy-bootstrap-path", "/path.json",
			"-envoy-sds-address", "localhost",
			"-consul-ca-cert-file", "/not-a-file",
		},
		output: "no such file or directory",
	}, {
		name:   "bearer-token-file-error",
		retVal: 1,
		args: []string{
			"-consul-http-address", "localhost",
			"-gateway-host", "localhost",
			"-gateway-name", "gateway",
			"-envoy-bootstrap-path", "/path.json",
			"-envoy-sds-address", "localhost",
			"-acl-auth-method", "no-auth-method",
			"-acl-bearer-token-file", "/notafile",
		},
		output: "error reading bearer token",
	}, {
		name:   "discovery-error",
		retVal: 1,
		args: []string{
			"-consul-http-address", "127.0.0.1",
			"-gateway-host", "localhost",
			"-gateway-name", "gateway",
			"-envoy-bootstrap-path", "/path.json",
			"-envoy-sds-address", "localhost",
		},
		output: "did not get state within time limit",
	}} {
		t.Run(test.name, func(t *testing.T) {
			os.Setenv("CONSUL_DYNAMIC_SERVER_DISCOVERY", "true")
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
