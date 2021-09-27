package testing

import (
	"bytes"
	"sync"
)

type Buffer struct {
	buffer bytes.Buffer
	mutex  sync.RWMutex
}

func (s *Buffer) Write(p []byte) (n int, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buffer.Write(p)
}

func (s *Buffer) String() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.buffer.String()
}

func (s *Buffer) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.buffer.Reset()
}
