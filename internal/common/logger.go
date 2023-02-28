// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"io"
	"os"

	"github.com/hashicorp/go-hclog"
)

func CreateLogger(output io.Writer, logLevel string, asJSON bool, name string) hclog.Logger {
	return hclog.New(&hclog.LoggerOptions{
		Level:           hclog.LevelFromString(logLevel),
		Output:          output,
		JSONFormat:      asJSON,
		IncludeLocation: true,
	}).Named(name)
}

func GetConsulTokenOr(tokenFlag string) string {
	if tokenFlag != "" {
		return tokenFlag
	}
	return os.Getenv("CONSUL_HTTP_TOKEN")
}
