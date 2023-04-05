// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"time"
)

const (
	defaultMaxRetries      = uint64(30)
	defaultBackoffInterval = 1 * time.Second
)
