package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
)

type heartbeatStore struct {
	jobs.Store
	mu       sync.Mutex
	calls    int
	failCall int
}

func (store *heartbeatStore) ExtendLease(
	context.Context,
	string,
	string,
	time.Time,
	time.Duration,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	if store.failCall == store.calls {
		return jobs.ErrConflict
	}
	return nil
}

func (store *heartbeatStore) callCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.calls
}

func TestLeaseHeartbeatExtendsUntilStopped(t *testing.T) {
	t.Parallel()

	store := &heartbeatStore{}
	ticks := make(chan time.Time)
	workCtx, cancelWork := context.WithCancel(context.Background())
	heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go runLeaseHeartbeat(
		heartbeatCtx, cancelWork, store, "job", "worker", time.Minute,
		func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
		ticks, done,
	)

	ticks <- time.Now()
	ticks <- time.Now()
	cancelHeartbeat()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if store.callCount() != 2 {
		t.Fatalf("ExtendLease calls = %d, want 2", store.callCount())
	}
	if err := workCtx.Err(); err != nil {
		t.Fatalf("work context was cancelled: %v", err)
	}
}

func TestLeaseHeartbeatCancelsWorkWhenOwnershipIsLost(t *testing.T) {
	t.Parallel()

	store := &heartbeatStore{failCall: 1}
	ticks := make(chan time.Time)
	workCtx, cancelWork := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go runLeaseHeartbeat(
		context.Background(), cancelWork, store, "job", "worker", time.Minute,
		time.Now, ticks, done,
	)

	ticks <- time.Now()
	if err := <-done; !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("heartbeat error = %v, want ErrConflict", err)
	}
	if !errors.Is(workCtx.Err(), context.Canceled) {
		t.Fatalf("work context error = %v, want cancelled", workCtx.Err())
	}
}

func TestLeaseHeartbeatStopsOnParentCancellation(t *testing.T) {
	t.Parallel()

	store := &heartbeatStore{}
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go runLeaseHeartbeat(
		ctx, func() {}, store, "job", "worker", time.Minute,
		time.Now, ticks, done,
	)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("heartbeat error = %v", err)
	}
	if store.callCount() != 0 {
		t.Fatalf("ExtendLease calls = %d, want 0", store.callCount())
	}
}
