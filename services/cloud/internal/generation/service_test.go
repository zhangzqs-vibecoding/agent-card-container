package generation_test

import (
	"context"
	"errors"
	"reflect"
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

func TestServiceConfirmFreezesCompleteRequirementAndSummary(t *testing.T) {
	t.Parallel()

	current := time.Date(2026, 7, 13, 1, 0, 0, 0, time.FixedZone("UTC-4", -4*60*60))
	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_complete" },
		func() time.Time { return current },
	)
	session, err := service.Create(context.Background(), "user", generation.CreateRequest{
		Prompt: "生成离线番茄钟",
		Target: generation.TargetNative,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	current = current.Add(time.Minute)
	if _, err := service.AddMessage(context.Background(), "user", session.ID, "增加暂停按钮"); err != nil {
		t.Fatalf("AddMessage(first) error = %v", err)
	}
	current = current.Add(time.Minute)
	updated, err := service.AddMessage(context.Background(), "user", session.ID, "使用中文显示")
	if err != nil {
		t.Fatalf("AddMessage(second) error = %v", err)
	}
	const wantGoal = "生成离线番茄钟\n增加暂停按钮\n使用中文显示"
	if updated.Summary.Goal != wantGoal {
		t.Fatalf("updated Summary.Goal = %q, want %q", updated.Summary.Goal, wantGoal)
	}
	if !reflect.DeepEqual(updated.Summary.Constraints, []string{
		"卡片必须通过能力代理访问宿主能力",
		"纯本地功能必须可离线使用",
	}) {
		t.Fatalf("updated Summary.Constraints = %#v", updated.Summary.Constraints)
	}

	current = current.Add(time.Minute)
	confirmed, err := service.Confirm(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if confirmed.Summary.Goal != wantGoal {
		t.Fatalf("confirmed Summary.Goal = %q, want %q", confirmed.Summary.Goal, wantGoal)
	}
	wantSnapshot := &generation.RequirementSnapshot{
		InitialPrompt: "生成离线番茄钟",
		AdditionalMessages: []generation.Message{
			{Role: "user", Content: "增加暂停按钮", CreatedAt: current.Add(-2 * time.Minute).UTC()},
			{Role: "user", Content: "使用中文显示", CreatedAt: current.Add(-time.Minute).UTC()},
		},
		Target:              generation.TargetNative,
		Locale:              "zh-CN",
		AllowedCapabilities: []string{"storage", "window.manageSelf"},
		ConfirmedAt:         current.UTC(),
	}
	if !reflect.DeepEqual(confirmed.ConfirmedRequirement, wantSnapshot) {
		t.Fatalf("ConfirmedRequirement = %#v, want %#v", confirmed.ConfirmedRequirement, wantSnapshot)
	}

	frozen := *confirmed.ConfirmedRequirement
	frozen.AdditionalMessages = append([]generation.Message(nil), confirmed.ConfirmedRequirement.AdditionalMessages...)
	frozen.AllowedCapabilities = append([]string(nil), confirmed.ConfirmedRequirement.AllowedCapabilities...)
	if _, err := service.Confirm(context.Background(), "user", session.ID); !errors.Is(err, generation.ErrConflict) {
		t.Fatalf("second Confirm() error = %v, want ErrConflict", err)
	}
	if _, err := service.AddMessage(context.Background(), "user", session.ID, "确认后追加"); !errors.Is(err, generation.ErrConflict) {
		t.Fatalf("AddMessage() after confirmation error = %v, want ErrConflict", err)
	}
	reloaded, err := service.Get(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(reloaded.ConfirmedRequirement, &frozen) {
		t.Fatalf("rejected updates changed snapshot to %#v, want %#v", reloaded.ConfirmedRequirement, frozen)
	}
}

func TestMemoryRepositoryDeepClonesConfirmedRequirement(t *testing.T) {
	t.Parallel()

	repository := generation.NewMemoryRepository()
	service := generation.NewService(
		repository,
		func() string { return "gen_clone" },
		func() time.Time { return time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC) },
	)
	session, err := service.Create(context.Background(), "user", generation.CreateRequest{
		Prompt: "生成待办卡片",
		Target: generation.TargetAuto,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := service.AddMessage(context.Background(), "user", session.ID, "支持离线保存"); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	confirmed, err := service.Confirm(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	confirmed.ConfirmedRequirement.AdditionalMessages[0].Content = "已篡改"
	confirmed.ConfirmedRequirement.AllowedCapabilities[0] = "network"

	firstRead, err := repository.Get(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("Get(first) error = %v", err)
	}
	if got := firstRead.ConfirmedRequirement.AdditionalMessages[0].Content; got != "支持离线保存" {
		t.Fatalf("stored AdditionalMessages[0].Content = %q", got)
	}
	if got := firstRead.ConfirmedRequirement.AllowedCapabilities[0]; got != "storage" {
		t.Fatalf("stored AllowedCapabilities[0] = %q", got)
	}
	firstRead.ConfirmedRequirement.AdditionalMessages[0].Content = "再次篡改"
	firstRead.ConfirmedRequirement.AllowedCapabilities[0] = "clipboard"
	secondRead, err := repository.Get(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatalf("Get(second) error = %v", err)
	}
	if got := secondRead.ConfirmedRequirement.AdditionalMessages[0].Content; got != "支持离线保存" {
		t.Fatalf("reloaded AdditionalMessages[0].Content = %q", got)
	}
	if got := secondRead.ConfirmedRequirement.AllowedCapabilities[0]; got != "storage" {
		t.Fatalf("reloaded AllowedCapabilities[0] = %q", got)
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

func TestServiceSubscribeEventsReplaysThenStreamsNewEvents(t *testing.T) {
	t.Parallel()

	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_stream" },
		time.Now,
	)
	session, err := service.Create(context.Background(), "user", generation.CreateRequest{
		Prompt: "离线时钟",
		Target: generation.TargetNative,
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeEvents(context.Background(), "user", session.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if first := <-events; first.EventID != 1 {
		t.Fatalf("first event ID = %d, want 1", first.EventID)
	}
	if _, err := service.Confirm(context.Background(), "user", session.ID); err != nil {
		t.Fatal(err)
	}
	second := <-events
	if second.EventID != 2 || second.Stage != "queued" {
		t.Fatalf("second event = %#v", second)
	}
}
