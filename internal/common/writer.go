// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"io"
	"sync"
)

type synchronizedWriter struct {
	io.Writer
	mutex sync.Mutex
}

func SynchronizeWriter(writer io.Writer) io.Writer {
	return &synchronizedWriter{Writer: writer}
}

func (w *synchronizedWriter) Write(p []byte) (n int, err error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.Writer.Write(p)
}
