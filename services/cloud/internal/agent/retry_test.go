package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProviderRetryDelayUsesShortExponentialBackoff(t *testing.T) {
	t.Parallel()

	if delay := providerRetryDelay(0); delay != 500*time.Millisecond {
		t.Fatalf("first retry delay = %s, want 500ms", delay)
	}
	if delay := providerRetryDelay(1); delay != time.Second {
		t.Fatalf("second retry delay = %s, want 1s", delay)
	}
}

func TestWaitForProviderRetryReturnsImmediatelyWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startedAt := time.Now()
	err := waitForProviderRetry(ctx, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForProviderRetry() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 50*time.Millisecond {
		t.Fatalf("cancelled retry wait took %s, want less than 50ms", elapsed)
	}
}
