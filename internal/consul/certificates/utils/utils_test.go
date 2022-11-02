package utils

import (
	"testing"
	"time"
)

func TestRandomStagger(t *testing.T) {
	intv := time.Minute
	for i := 0; i < 10; i++ {
		stagger := RandomStagger(intv)
		if stagger < 0 || stagger >= intv {
			t.Fatalf("Bad: %v", stagger)
		}
	}
}
