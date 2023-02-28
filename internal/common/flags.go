// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import "strings"

type ArrayFlag []string

func (i *ArrayFlag) String() string {
	return strings.Join(*i, ", ")
}

func (i *ArrayFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}
