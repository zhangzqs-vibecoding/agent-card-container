package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
)

func TestMemoryStoreEnqueuesIdempotentlyAndClaimsOnce(t *testing.T) {
	t.Parallel()

	store := jobs.NewMemoryStore(func() string { return "job_01" })
	now := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	first, err := store.Enqueue(context.Background(), "gen_01", now)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.Enqueue(context.Background(), "gen_01", now)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != repeated.ID {
		t.Fatalf("job IDs = %q, %q", first.ID, repeated.ID)
	}

	var wait sync.WaitGroup
	results := make(chan *jobs.Job, 2)
	for _, worker := range []string{"worker-a", "worker-b"} {
		wait.Add(1)
		go func(worker string) {
			defer wait.Done()
			job, claimErr := store.Claim(context.Background(), worker, now, time.Minute)
			if claimErr != nil && !errors.Is(claimErr, jobs.ErrNoJob) {
				t.Errorf("Claim() error = %v", claimErr)
				return
			}
			results <- job
		}(worker)
	}
	wait.Wait()
	close(results)

	claimed := 0
	for job := range results {
		if job != nil {
			claimed++
			if job.Attempts != 1 || job.Status != jobs.StatusRunning {
				t.Fatalf("claimed job = %#v", job)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed = %d, want 1", claimed)
	}
}

func TestMemoryStoreReclaimsExpiredLeaseAndStopsAfterThreeAttempts(t *testing.T) {
	t.Parallel()

	counter := 0
	store := jobs.NewMemoryStore(func() string {
		counter++
		return "job_01"
	})
	now := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	if _, err := store.Enqueue(context.Background(), "gen_01", now); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		job, err := store.Claim(context.Background(), "worker", now, time.Second)
		if err != nil {
			t.Fatalf("Claim(attempt %d) error = %v", attempt, err)
		}
		if job.Attempts != attempt {
			t.Fatalf("Attempts = %d, want %d", job.Attempts, attempt)
		}
		now = now.Add(2 * time.Second)
	}

	if _, err := store.Claim(context.Background(), "worker", now, time.Second); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatalf("fourth Claim() error = %v, want ErrNoJob", err)
	}
	job, err := store.Get(context.Background(), "job_01")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusFailed {
		t.Fatalf("Status = %q, want failed", job.Status)
	}
}

func TestMemoryStoreCompletesAndCancelsIdempotently(t *testing.T) {
	t.Parallel()

	store := jobs.NewMemoryStore(func() string { return "job_01" })
	now := time.Now().UTC()
	job, err := store.Enqueue(context.Background(), "gen_01", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(context.Background(), job.ID, "worker", now); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(context.Background(), job.ID, "worker", now); err != nil {
		t.Fatalf("repeated Complete() error = %v", err)
	}

	cancelled, err := store.Enqueue(context.Background(), "gen_02", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CancelBySession(context.Background(), "gen_02", now); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), cancelled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != jobs.StatusCancelled {
		t.Fatalf("Status = %q, want cancelled", got.Status)
	}
}

func TestMemoryStoreExtendsOnlyCurrentUnexpiredLease(t *testing.T) {
	t.Parallel()

	store := jobs.NewMemoryStore(func() string { return "job_lease" })
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	job, err := store.Enqueue(context.Background(), "gen_lease", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker-a", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.ExtendLease(context.Background(), job.ID, "worker-a", now.Add(30*time.Second), time.Minute); err != nil {
		t.Fatal(err)
	}
	extended, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantUntil := now.Add(90 * time.Second)
	if !extended.LeaseUntil.Equal(wantUntil) {
		t.Fatalf("LeaseUntil = %s, want %s", extended.LeaseUntil, wantUntil)
	}
	if err := store.ExtendLease(context.Background(), job.ID, "worker-b", now.Add(40*time.Second), time.Minute); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("wrong owner ExtendLease() error = %v", err)
	}
	if err := store.ExtendLease(context.Background(), job.ID, "worker-a", wantUntil, time.Minute); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("expired ExtendLease() error = %v", err)
	}
	if err := store.Complete(context.Background(), job.ID, "worker-a", wantUntil); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("expired Complete() error = %v", err)
	}
}

func TestMemoryStoreReservesStablePublicationAcrossAttempts(t *testing.T) {
	t.Parallel()

	store := jobs.NewMemoryStore(func() string { return "job_publish" })
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	job, err := store.Enqueue(context.Background(), "gen_publish", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker-a", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	first, err := store.ReservePublication(
		context.Background(), job.ID, "worker-a", "card_first", "ver_first", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.ReservePublication(
		context.Background(), job.ID, "worker-a", "card_other", "ver_other", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.CardID != "card_first" || first.VersionID != "ver_first" || repeated != first {
		t.Fatalf("publications = %#v, %#v", first, repeated)
	}
}

func TestMemoryStoreRetriesOnlyAfterAvailableAt(t *testing.T) {
	t.Parallel()

	store := jobs.NewMemoryStore(func() string { return "job_retry" })
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	job, err := store.Enqueue(context.Background(), "gen_retry", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker-a", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	availableAt := now.Add(5 * time.Second)
	if err := store.Retry(context.Background(), job.ID, "worker-a", now.Add(time.Second), availableAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker-b", availableAt.Add(-time.Nanosecond), time.Minute); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatalf("early Claim() error = %v", err)
	}
	retried, err := store.Claim(context.Background(), "worker-b", availableAt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Attempts != 2 || retried.LeaseOwner != "worker-b" {
		t.Fatalf("retried job = %#v", retried)
	}
}
