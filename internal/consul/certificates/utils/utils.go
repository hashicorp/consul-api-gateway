package utils

import (
	"math/rand"
	"time"
)

// Constants related to cache refresh backoff. We probably don't ever need to
// make these configurable knobs since they primarily exist to lower load.
const (
	BackoffMin     = 3               // 3 attempts before backing off
	BackoffMaxWait = 1 * time.Minute // maximum backoff wait time
)

// RandomStagger returns an interval between 0 and the duration
func RandomStagger(intv time.Duration) time.Duration {
	if intv == 0 {
		return 0
	}
	return time.Duration(uint64(rand.Int63()) % uint64(intv))
}

func BackOffWait(failures uint) time.Duration {
	if failures > BackoffMin {
		shift := failures - BackoffMin
		waitTime := BackoffMaxWait
		if shift < 31 {
			waitTime = (1 << shift) * time.Second
		}
		if waitTime > BackoffMaxWait {
			waitTime = BackoffMaxWait
		}
		return waitTime + RandomStagger(waitTime)
	}
	return 0
}
