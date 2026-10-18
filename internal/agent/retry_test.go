package agent

import (
	"context"
	"testing"
	"time"
)

func TestRetryBoundedAndCancellable(t *testing.T) {
	for i := 0; i < 100; i++ {
		base := time.Second << min(max(i-1, 0), 5)
		d := RetryDelay(i)
		if d < base || d > base+base/2 || d > 48*time.Second {
			t.Fatal("unbounded backoff")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Wait(ctx, time.Minute) {
		t.Fatal("shutdown ignored")
	}
}
