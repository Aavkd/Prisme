package agent

import (
	"context"
	"math/rand/v2"
	"time"
)

// Retry observations only; terminal input and infrastructure mutations must
// never use this policy. Persisted enrollment CSRs make Connect replay-safe.
func RetryDelay(failures int) time.Duration {
	base := time.Second << min(max(failures-1, 0), 5)
	return base + time.Duration(rand.Int64N(int64(base/2)+1))
}

func Wait(ctx context.Context, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
