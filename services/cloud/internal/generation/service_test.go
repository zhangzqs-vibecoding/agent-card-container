package generation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
)

func TestServiceCreatesConfirmsAndReplaysGeneration(t *testing.T) {
	t.Parallel()

	clock := func() time.Time {
		return time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	}
	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_01" },
		clock,
	)

	session, err := service.Create(context.Background(), "user_01", generation.CreateRequest{
		Prompt: "做一个离线番茄钟",
		Target: generation.TargetAuto,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.Status != generation.StatusAwaitingConfirmation {
		t.Fatalf("Status = %q, want awaiting_confirmation", session.Status)
	}
	if session.Summary.Goal == "" {
		t.Fatal("Summary.Goal is empty")
	}

	if _, err := service.AddMessage(context.Background(), "user_01", session.ID, "需要有暂停按钮"); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	confirmed, err := service.Confirm(context.Background(), "user_01", session.ID)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if confirmed.Status != generation.StatusQueued {
		t.Fatalf("Status = %q, want queued", confirmed.Status)
	}

	events, err := service.EventsAfter(context.Background(), "user_01", session.ID, 1)
	if err != nil {
		t.Fatalf("EventsAfter() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].EventID != 2 || events[1].EventID != 3 {
		t.Fatalf("event IDs = %d, %d, want 2, 3", events[0].EventID, events[1].EventID)
	}
}

func TestServiceEnforcesOwnershipAndConflicts(t *testing.T) {
	t.Parallel()

	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_01" },
		time.Now,
	)
	session, err := service.Create(context.Background(), "owner", generation.CreateRequest{
		Prompt: "做一个待办",
		Target: generation.TargetNative,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Get(context.Background(), "other", session.ID); !errors.Is(err, generation.ErrNotFound) {
		t.Fatalf("Get(other) error = %v, want ErrNotFound", err)
	}
	if _, err := service.Confirm(context.Background(), "owner", session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "owner", session.ID); !errors.Is(err, generation.ErrConflict) {
		t.Fatalf("second Confirm() error = %v, want ErrConflict", err)
	}
}

func TestServiceCancelsQueuedSessionIdempotently(t *testing.T) {
	t.Parallel()

	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_01" },
		time.Now,
	)
	session, err := service.Create(context.Background(), "user", generation.CreateRequest{
		Prompt: "做一个卡片",
		Target: generation.TargetAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "user", session.ID); err != nil {
		t.Fatal(err)
	}

	cancelled, err := service.Cancel(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	repeated, err := service.Cancel(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("repeated Cancel() error = %v", err)
	}
	if cancelled.Status != generation.StatusCancelled || repeated.Status != generation.StatusCancelled {
		t.Fatalf("statuses = %q, %q", cancelled.Status, repeated.Status)
	}
}

func TestServiceConfirmEnqueuesAndCancelStopsJob(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := jobs.NewMemoryStore(func() string { return "job_01" })
	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_01" },
		func() time.Time { return now },
		generation.WithJobQueue(jobs.NewGenerationQueue(store)),
	)
	session, err := service.Create(context.Background(), "user", generation.CreateRequest{
		Prompt: "生成卡片",
		Target: generation.TargetAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "user", session.ID); err != nil {
		t.Fatal(err)
	}
	job, err := store.Claim(context.Background(), "worker", now, time.Minute)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if job.SessionID != session.ID {
		t.Fatalf("job session = %q", job.SessionID)
	}
	if _, err := service.Cancel(context.Background(), "user", session.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != jobs.StatusCancelled {
		t.Fatalf("job status = %q", cancelled.Status)
	}
}
