// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testing

import "math/rand"

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

func RandomString() string {
	s := make([]rune, 10)
	for i := range s {
		s[i] = letters[rand.Intn(26)]
	}
	return string(s)
}

func StringPtr(val string) *string {
	return &val
}
