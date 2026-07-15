package worker_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/worker"
)

func TestWorkerCancelsGenerationWhenHeartbeatLosesLease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC()
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_lease_loss" }, time.Now)
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "生成离线卡片", Target: generation.TargetNative, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	baseStore := jobs.NewMemoryStore(func() string { return "job_lease_loss" })
	if _, err := baseStore.Enqueue(ctx, session.ID, now); err != nil {
		t.Fatal(err)
	}
	jobStore := &leaseLosingStore{Store: baseStore}
	seed := sha256.Sum256([]byte("lease-loss-key"))
	runner := worker.New(worker.Config{
		WorkerID: "worker-lease-loss", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(blockingProvider{}, agent.NewNativeValidator()),
		Publisher: publish.NewPublisher(
			artifact.NewBuilder("lease-loss-key", ed25519.NewKeyFromSeed(seed[:])),
			publish.NewMemoryObjectStore(), publish.NewMemoryVersionRepository(),
		),
		NewCardID: func() string { return "card_lease_loss" }, NewVersionID: func() string { return "ver_lease_loss" },
		Now: time.Now, LeaseDuration: 20 * time.Millisecond, HeartbeatInterval: 5 * time.Millisecond,
	})

	if _, err := runner.RunOnce(ctx); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("RunOnce() error = %v, want lease conflict", err)
	}
	stored, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != generation.StatusGenerating || stored.VersionID != "" {
		t.Fatalf("session after lease loss = %#v", stored)
	}
}

func TestWorkerUsesOnlyConfirmedRequirementSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_snapshot" }, func() time.Time { return now })
	session, err := service.Create(ctx, "user_snapshot", generation.CreateRequest{
		Prompt: "初始需求：离线文本卡片",
		Target: generation.TargetNative,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"补充一：显示当前日期", "补充二：使用大号字体"} {
		if _, err := service.AddMessage(ctx, "user_snapshot", session.ID, message); err != nil {
			t.Fatal(err)
		}
	}
	confirmed, err := service.Confirm(ctx, "user_snapshot", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedPrompt := strings.Join([]string{
		"Session ID: gen_snapshot",
		"Runtime: native",
		"Attempt: 1",
		"Initial requirement:",
		confirmed.ConfirmedRequirement.InitialPrompt,
		"Additional requirements:",
		"1. " + confirmed.ConfirmedRequirement.AdditionalMessages[0].Content,
		"2. " + confirmed.ConfirmedRequirement.AdditionalMessages[1].Content,
		"Target: native",
		"Locale: zh-CN",
		"Allowed capabilities:",
		"- storage",
		"- window.manageSelf",
	}, "\n")
	expectedDescription := strings.Join([]string{
		confirmed.ConfirmedRequirement.InitialPrompt,
		confirmed.ConfirmedRequirement.AdditionalMessages[0].Content,
		confirmed.ConfirmedRequirement.AdditionalMessages[1].Content,
	}, "\n")
	if _, err := repository.UpdateSystem(ctx, session.ID, func(candidate *generation.Session) error {
		candidate.Prompt = "legacy-prompt-tampered：自由绘制画板"
		candidate.Target = generation.TargetAuto
		candidate.Locale = "legacy-locale-tampered"
		candidate.Summary.Goal = "legacy-summary-tampered"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_snapshot" })
	if _, err := jobStore.Enqueue(ctx, session.ID, now); err != nil {
		t.Fatal(err)
	}
	provider := &captureProvider{}
	objectStore := &captureObjectStore{}
	runner := newWorkerForTest(
		now,
		service,
		jobStore,
		agent.NewCodingAgent(provider, agent.NewNativeValidator(), agent.WithWebBuilder(staticWebBuilder{})),
		objectStore,
	)

	if _, err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.requests))
	}
	request := provider.requests[0]
	if request.UserPrompt != expectedPrompt {
		t.Fatalf("provider prompt = %q, want %q", request.UserPrompt, expectedPrompt)
	}
	if !strings.Contains(request.SystemPrompt, "NativeCard") || !request.JSONOutput || request.MaxTokens != 8192 {
		t.Fatalf("provider transport = %#v", request)
	}
	manifest := decodeManifest(t, objectStore.archive)
	if manifest.Description != expectedDescription {
		t.Fatalf("manifest description = %q, want snapshot description %q", manifest.Description, expectedDescription)
	}
	if strings.Contains(manifest.Description, "legacy-summary-tampered") {
		t.Fatalf("manifest description contains tampered legacy summary: %q", manifest.Description)
	}
	for _, tampered := range []string{
		"legacy-prompt-tampered",
		"legacy-locale-tampered",
		"legacy-summary-tampered",
	} {
		if strings.Contains(request.UserPrompt, tampered) {
			t.Fatalf("provider request contains tampered legacy value %q: %#v", tampered, request)
		}
	}
}

func TestWorkerPublishesCapabilitiesFromConfirmedRequirementSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_metadata" }, func() time.Time { return now })
	session, err := service.Create(ctx, "user_metadata", generation.CreateRequest{
		Prompt: "初始需求：天气概览",
		Target: generation.TargetNative,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"补充一：显示体感温度", "补充二：完全离线"} {
		if _, err := service.AddMessage(ctx, "user_metadata", session.ID, message); err != nil {
			t.Fatal(err)
		}
	}
	confirmed, err := service.Confirm(ctx, "user_metadata", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedCapabilities := []string{"notification.show", "storage"}
	if _, err := repository.UpdateSystem(ctx, session.ID, func(candidate *generation.Session) error {
		candidate.ConfirmedRequirement.AllowedCapabilities = append([]string(nil), expectedCapabilities...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_metadata" })
	if _, err := jobStore.Enqueue(ctx, session.ID, now); err != nil {
		t.Fatal(err)
	}
	objectStore := &captureObjectStore{}
	runner := newWorkerForTest(
		now,
		service,
		jobStore,
		agent.NewCodingAgent(&captureProvider{}, agent.NewNativeValidator()),
		objectStore,
	)

	if _, err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	manifest := decodeManifest(t, objectStore.archive)
	if manifest.Title != confirmed.ConfirmedRequirement.InitialPrompt {
		t.Fatalf("manifest title = %q, want %q", manifest.Title, confirmed.ConfirmedRequirement.InitialPrompt)
	}
	expectedDescription := "初始需求：天气概览\n补充一：显示体感温度\n补充二：完全离线"
	if manifest.Description != expectedDescription || confirmed.Summary.Goal != expectedDescription {
		t.Fatalf(
			"manifest description = %q, confirmed summary = %q, want %q",
			manifest.Description,
			confirmed.Summary.Goal,
			expectedDescription,
		)
	}
	if !slices.Equal(manifest.Capabilities, expectedCapabilities) {
		t.Fatalf("manifest capabilities = %#v, want %#v", manifest.Capabilities, expectedCapabilities)
	}
	expectedCapabilities[0] = "tampered-after-publish"
	if manifest.Capabilities[0] != "notification.show" {
		t.Fatalf("manifest capabilities alias test input: %#v", manifest.Capabilities)
	}
}

func TestWorkerRejectsMissingConfirmedRequirement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)
	repository := generation.NewMemoryRepository()
	session, err := generation.NewSession(generation.CreateInput{
		ID:        "gen_legacy",
		UserID:    "user_legacy",
		Prompt:    "旧会话没有冻结快照",
		Target:    generation.TargetNative,
		Locale:    "zh-CN",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Transition(generation.StatusAwaitingConfirmation, generation.Transition{At: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Transition(generation.StatusQueued, generation.Transition{At: now}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	service := generation.NewService(repository, func() string { return "unused" }, func() time.Time { return now })
	jobStore := jobs.NewMemoryStore(func() string { return "job_legacy" })
	if _, err := jobStore.Enqueue(ctx, session.ID, now); err != nil {
		t.Fatal(err)
	}
	provider := &captureProvider{}
	runner := newWorkerForTest(
		now,
		service,
		jobStore,
		agent.NewCodingAgent(provider, agent.NewNativeValidator()),
		publish.NewMemoryObjectStore(),
	)

	_, err = runner.RunOnce(ctx)
	if err == nil || !strings.Contains(err.Error(), "GENERATION_REQUIREMENTS_MISSING") {
		t.Fatalf("RunOnce() error = %v, want GENERATION_REQUIREMENTS_MISSING", err)
	}
	failed, err := service.Get(ctx, "user_legacy", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != generation.StatusFailed {
		t.Fatalf("session status = %q, want failed", failed.Status)
	}
	lastEvent := failed.Events[len(failed.Events)-1]
	if lastEvent.ErrorCode != "GENERATION_REQUIREMENTS_MISSING" {
		t.Fatalf("error code = %q, want GENERATION_REQUIREMENTS_MISSING", lastEvent.ErrorCode)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(provider.requests))
	}
}

func TestWorkerPublishesValidatedNativeCardAndMarksReady(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 12, 13, 0, 0, 0, time.UTC)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_01" }, func() time.Time { return now })
	session, err := service.Create(context.Background(), "user_01", generation.CreateRequest{
		Prompt: "做一个离线文本卡片",
		Target: generation.TargetAuto,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "user_01", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_01" })
	if _, err := jobStore.Enqueue(context.Background(), session.ID, now); err != nil {
		t.Fatal(err)
	}
	provider := staticProvider{
		content: `{"schemaVersion":1,"initialState":{"text":"完成"},"root":{"id":"root","type":"Text","props":{"text":{"path":"state.text"}}}}`,
	}
	var logs bytes.Buffer
	logger := observability.NewJSONLogger(&logs)
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator(), agent.WithLogger(logger))
	seed := sha256.Sum256([]byte("worker-signing-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("release-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(),
		publish.NewMemoryVersionRepository(),
	)
	runner := worker.New(worker.Config{
		WorkerID:     "worker-01",
		Jobs:         jobStore,
		Generations:  service,
		Agent:        codingAgent,
		Publisher:    publisher,
		NewCardID:    func() string { return "card_01" },
		NewVersionID: func() string { return "ver_01" },
		Now:          func() time.Time { return now },
		Logger:       logger,
	})

	outcome, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if outcome.VersionID != "ver_01" {
		t.Fatalf("outcome = %#v", outcome)
	}
	ready, err := service.Get(context.Background(), "user_01", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != generation.StatusReady || ready.VersionID != "ver_01" {
		t.Fatalf("session = %#v", ready)
	}
	job, err := jobStore.Get(context.Background(), "job_01")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("job status = %q", job.Status)
	}
	if _, err := publisher.Download(context.Background(), "user_01", "card_01", "ver_01", time.Minute); err != nil {
		t.Fatalf("published artifact unavailable: %v", err)
	}
	for _, event := range []string{"generation_job_started", "artifact_publish_started", "artifact_publish_completed", "generation_job_completed"} {
		if !strings.Contains(logs.String(), `"event":"`+event+`"`) {
			t.Fatalf("missing %s in logs: %s", event, logs.String())
		}
	}
	if strings.Contains(logs.String(), "做一个离线文本卡片") || strings.Contains(logs.String(), "initialState") {
		t.Fatalf("sensitive generation content leaked: %s", logs.String())
	}
}

func TestWorkerUsesPublicationIdentityReservedByEarlierAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	firstAttemptAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	retryAt := firstAttemptAt.Add(2 * time.Minute)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_stable" }, func() time.Time { return retryAt })
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "稳定发布", Target: generation.TargetNative, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_stable" })
	job, err := jobStore.Enqueue(ctx, session.ID, firstAttemptAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobStore.Claim(ctx, "worker-old", firstAttemptAt, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := jobStore.ReservePublication(
		ctx, job.ID, "worker-old", "card_reserved", "ver_reserved", firstAttemptAt,
	); err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("stable-publication-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("stable-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(), publish.NewMemoryVersionRepository(),
	)
	runner := worker.New(worker.Config{
		WorkerID: "worker-new", Jobs: jobStore, Generations: service,
		Agent:     agent.NewCodingAgent(&captureProvider{}, agent.NewNativeValidator()),
		Publisher: publisher,
		NewCardID: func() string { return "card_new_candidate" }, NewVersionID: func() string { return "ver_new_candidate" },
		Now: func() time.Time { return retryAt },
	})

	outcome, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.VersionID != "ver_reserved" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if _, err := publisher.Card(ctx, "user", "card_reserved"); err != nil {
		t.Fatalf("reserved card was not published: %v", err)
	}
	if _, err := publisher.Card(ctx, "user", "card_new_candidate"); !errors.Is(err, publish.ErrNotFound) {
		t.Fatalf("candidate card lookup error = %v", err)
	}
}

func TestWorkerPublishesVerifiedIterationOnExistingCard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
	seed := sha256.Sum256([]byte("iteration-worker-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	objects := publish.NewMemoryObjectStore()
	versions := publish.NewMemoryVersionRepository()
	publisher := publish.NewPublisher(artifact.NewBuilder("iteration-key", privateKey), objects, versions)
	baseDefinition := contracts.CardDefinition{
		FormatVersion: 1, MinHostVersion: "1.0.0", CardID: "card_existing", VersionID: "ver_base",
		DisplayVersion: "1.0.0", Runtime: contracts.CardRuntimeNative, StateSchemaVersion: 3,
		Title: "旧版", Entrypoint: "payload/native.json", CatalogVersion: "1",
		MinSize: contracts.Size{Width: 200, Height: 120}, PreferredSize: contracts.Size{Width: 420, Height: 260},
		MaxSize: contracts.Size{Width: 900, Height: 700}, Capabilities: []string{"storage"},
		NetworkPolicy: contracts.NetworkPolicy{Mode: "none", Domains: []string{}}, CreatedAt: now.Add(-time.Hour),
	}
	if _, err := publisher.Publish(ctx, publish.Input{
		UserID: "owner", Definition: baseDefinition, CreatedAt: baseDefinition.CreatedAt,
		Files: map[string][]byte{
			"payload/native.json": []byte(`{"schemaVersion":1,"initialState":{"count":1},"root":{"id":"root","type":"Text"}}`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	service := generation.NewService(
		generation.NewMemoryRepository(), func() string { return "gen_iteration" }, func() time.Time { return now },
		generation.WithBaseVersionCatalog(publisher),
	)
	session, err := service.Create(ctx, "owner", generation.CreateRequest{
		Prompt: "增加重置按钮", Target: generation.TargetNative, Locale: "zh-CN",
		BaseCardID: "card_existing", BaseVersionID: "ver_base",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "owner", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_iteration" })
	if _, err := jobStore.Enqueue(ctx, session.ID, now); err != nil {
		t.Fatal(err)
	}
	provider := &captureProvider{}
	newCardCalls := 0
	runner := worker.New(worker.Config{
		WorkerID: "worker-iteration", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()), Publisher: publisher,
		TrustedArtifactKeys: map[string]ed25519.PublicKey{"iteration-key": publicKey},
		NewCardID:           func() string { newCardCalls++; return "card_wrong" },
		NewVersionID:        func() string { return "ver_next" }, Now: func() time.Time { return now },
	})
	outcome, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if outcome.VersionID != "ver_next" || newCardCalls != 0 || len(provider.requests) != 1 {
		t.Fatalf("outcome=%#v newCardCalls=%d providerCalls=%d", outcome, newCardCalls, len(provider.requests))
	}
	if !strings.Contains(provider.requests[0].UserPrompt, "Existing signed base version") ||
		!strings.Contains(provider.requests[0].UserPrompt, `\"count\":1`) {
		t.Fatalf("iteration prompt = %q", provider.requests[0].UserPrompt)
	}
	detail, err := publisher.Card(ctx, "owner", "card_existing")
	if err != nil || len(detail.Versions) != 2 {
		t.Fatalf("card detail = %#v, %v", detail, err)
	}
	var next publish.CardVersion
	for _, version := range detail.Versions {
		if version.VersionID == "ver_next" {
			next = version
		}
	}
	if next.DisplayVersion != "1.0.1" {
		t.Fatalf("next version = %#v", next)
	}
	loaded, err := publisher.LoadVersionArtifact(ctx, "owner", "card_existing", "ver_next", 8*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := agent.ParseBaseArtifact(loaded.Archive, agent.BaseArtifactExpectation{
		CardID: "card_existing", VersionID: "ver_next", KeyID: "iteration-key",
	}, map[string]ed25519.PublicKey{"iteration-key": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Definition.Runtime != baseDefinition.Runtime ||
		parsed.Definition.StateSchemaVersion != baseDefinition.StateSchemaVersion ||
		!slices.Equal(parsed.Definition.Capabilities, baseDefinition.Capabilities) ||
		parsed.Definition.NetworkPolicy.Mode != baseDefinition.NetworkPolicy.Mode {
		t.Fatalf("iteration definition did not inherit baseline: %#v", parsed.Definition)
	}
}

func TestWorkerCompletesVersionPublishedBeforePreviousAttemptCrashed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	firstAttemptAt := time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)
	retryAt := firstAttemptAt.Add(2 * time.Minute)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_recover" }, func() time.Time { return retryAt })
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "恢复发布", Target: generation.TargetNative, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartGenerating(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartValidating(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_recover" })
	job, err := jobStore.Enqueue(ctx, session.ID, firstAttemptAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobStore.Claim(ctx, "worker-old", firstAttemptAt, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := jobStore.ReservePublication(
		ctx, job.ID, "worker-old", "card_recover", "ver_recover", firstAttemptAt,
	); err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("recover-publication-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("recover-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(), publish.NewMemoryVersionRepository(),
	)
	definition := contracts.CardDefinition{
		FormatVersion: 1, MinHostVersion: "1.0.0", CardID: "card_recover", VersionID: "ver_recover",
		DisplayVersion: "1.0.0", Runtime: contracts.CardRuntimeNative, StateSchemaVersion: 1,
		Title: "恢复发布", Entrypoint: "payload/native.json", CatalogVersion: "1",
		MinSize: contracts.Size{Width: 240, Height: 160}, PreferredSize: contracts.Size{Width: 360, Height: 240}, MaxSize: contracts.Size{Width: 1200, Height: 900},
		Capabilities: []string{"storage", "window.manageSelf"}, NetworkPolicy: contracts.NetworkPolicy{Mode: "none", Domains: []string{}}, CreatedAt: firstAttemptAt,
	}
	if _, err := publisher.Publish(ctx, publish.Input{
		UserID: "user", Definition: definition, CreatedAt: firstAttemptAt,
		Files: map[string][]byte{
			"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{"text":"完成"},"root":{"id":"root","type":"Text","props":{"text":{"path":"state.text"}}}}`),
			"reports/validation.json": []byte(`{"status":"passed","runtime":"native","attempts":1}`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	provider := &captureProvider{}
	runner := worker.New(worker.Config{
		WorkerID: "worker-new", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()), Publisher: publisher,
		NewCardID: func() string { return "card_other" }, NewVersionID: func() string { return "ver_other" },
		Now: func() time.Time { return retryAt },
	})

	outcome, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.VersionID != "ver_recover" || len(provider.requests) != 0 {
		t.Fatalf("outcome = %#v, provider calls = %d", outcome, len(provider.requests))
	}
	ready, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := jobStore.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != generation.StatusReady || ready.VersionID != "ver_recover" || completed.Status != jobs.StatusCompleted {
		t.Fatalf("ready = %#v, job = %#v", ready, completed)
	}
}

func TestWorkerCompletesReadySessionAfterPreviousCompleteFailed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	current := time.Date(2026, 7, 15, 11, 30, 0, 0, time.UTC)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_complete_recovery" }, func() time.Time { return current })
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "完成恢复", Target: generation.TargetNative, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	baseStore := jobs.NewMemoryStore(func() string { return "job_complete_recovery" })
	if _, err := baseStore.Enqueue(ctx, session.ID, current); err != nil {
		t.Fatal(err)
	}
	jobStore := &completeFailOnceStore{Store: baseStore}
	provider := &captureProvider{}
	seed := sha256.Sum256([]byte("complete-recovery-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("complete-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(), publish.NewMemoryVersionRepository(),
	)
	runner := worker.New(worker.Config{
		WorkerID: "worker-complete", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()), Publisher: publisher,
		NewCardID: func() string { return "card_complete" }, NewVersionID: func() string { return "ver_complete" },
		Now: func() time.Time { return current },
	})

	if _, err := runner.RunOnce(ctx); !errors.Is(err, publish.ErrRetryable) {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	ready, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != generation.StatusReady || ready.VersionID != "ver_complete" {
		t.Fatalf("session = %#v", ready)
	}
	current = current.Add(2 * time.Minute)
	outcome, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.VersionID != "ver_complete" || len(provider.requests) != 1 {
		t.Fatalf("outcome=%#v provider calls=%d", outcome, len(provider.requests))
	}
	completed, err := baseStore.Get(ctx, "job_complete_recovery")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != jobs.StatusCompleted {
		t.Fatalf("job status = %s", completed.Status)
	}
}

func TestWorkerCancellationStopsBlockedProviderWithoutOverwritingSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	jobStore := jobs.NewMemoryStore(func() string { return "job_user_cancel" })
	service := generation.NewService(
		generation.NewMemoryRepository(), func() string { return "gen_user_cancel" }, time.Now,
		generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
	)
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "可取消生成", Target: generation.TargetNative, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	provider := &cancellationProvider{started: make(chan struct{}), cancelled: make(chan struct{})}
	seed := sha256.Sum256([]byte("user-cancel-key"))
	versions := publish.NewMemoryVersionRepository()
	runner := worker.New(worker.Config{
		WorkerID: "worker-user-cancel", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()),
		Publisher: publish.NewPublisher(
			artifact.NewBuilder("cancel-key", ed25519.NewKeyFromSeed(seed[:])),
			publish.NewMemoryObjectStore(), versions,
		),
		NewCardID: func() string { return "card_cancel" }, NewVersionID: func() string { return "ver_cancel" },
		Now: time.Now, LeaseDuration: 40 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond,
	})
	done := make(chan error, 1)
	go func() {
		_, runErr := runner.RunOnce(ctx)
		done <- runErr
	}()
	<-provider.started
	if _, err := service.Cancel(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.cancelled:
	case <-time.After(time.Second):
		t.Fatal("provider did not observe cancellation")
	}
	if err := <-done; !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("RunOnce() error = %v, want lease conflict", err)
	}
	cancelled, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobStore.Get(ctx, "job_user_cancel")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != generation.StatusCancelled || cancelled.VersionID != "" || job.Status != jobs.StatusCancelled {
		t.Fatalf("cancelled session=%#v job=%#v", cancelled, job)
	}
	if versionsCount, err := versions.ListByUser(ctx, "user"); err != nil || len(versionsCount) != 0 {
		t.Fatalf("versions after cancellation = %#v, error=%v", versionsCount, err)
	}
}

func TestWorkerFailsSessionAfterAgentValidationFailure(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	service := generation.NewService(generation.NewMemoryRepository(), func() string { return "gen_fail" }, func() time.Time { return now })
	session, err := service.Create(context.Background(), "user", generation.CreateRequest{Prompt: "卡片", Target: generation.TargetNative})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "user", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_fail" })
	if _, err := jobStore.Enqueue(context.Background(), session.ID, now); err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("key"))
	var logs bytes.Buffer
	logger := observability.NewJSONLogger(&logs)
	runner := worker.New(worker.Config{
		WorkerID:     "worker",
		Jobs:         jobStore,
		Generations:  service,
		Agent:        agent.NewCodingAgent(staticProvider{content: "{}"}, agent.NewNativeValidator(), agent.WithLogger(logger)),
		Publisher:    publish.NewPublisher(artifact.NewBuilder("key", ed25519.NewKeyFromSeed(seed[:])), publish.NewMemoryObjectStore(), publish.NewMemoryVersionRepository()),
		NewCardID:    func() string { return "card_fail" },
		NewVersionID: func() string { return "ver_fail" },
		Now:          func() time.Time { return now },
		Logger:       logger,
	})

	if _, err := runner.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce() succeeded with invalid model output")
	}
	failed, err := service.Get(context.Background(), "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != generation.StatusFailed {
		t.Fatalf("status = %q, want failed", failed.Status)
	}
	if !strings.Contains(logs.String(), `"event":"generation_job_failed"`) ||
		!strings.Contains(logs.String(), `"errorKind":"validation_failed"`) {
		t.Fatalf("missing safe failure event: %s", logs.String())
	}
	if strings.Contains(logs.String(), "VALIDATION_FAILED:") {
		t.Fatalf("raw error leaked: %s", logs.String())
	}
}

func TestWorkerRetriesTemporaryPublishFailureWithStableVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	current := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repository := generation.NewMemoryRepository()
	service := generation.NewService(repository, func() string { return "gen_retry_publish" }, func() time.Time { return current })
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "重试发布", Target: generation.TargetNative, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_retry_publish" })
	if _, err := jobStore.Enqueue(ctx, session.ID, current); err != nil {
		t.Fatal(err)
	}
	provider := &captureProvider{}
	objectStore := &temporaryFailureObjectStore{MemoryObjectStore: publish.NewMemoryObjectStore()}
	versions := publish.NewMemoryVersionRepository()
	seed := sha256.Sum256([]byte("retry-publish-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("retry-key", ed25519.NewKeyFromSeed(seed[:])), objectStore, versions,
	)
	runner := worker.New(worker.Config{
		WorkerID: "worker-retry", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()), Publisher: publisher,
		NewCardID: func() string { return "card_retry" }, NewVersionID: func() string { return "ver_retry" },
		Now: func() time.Time { return current },
	})

	if _, err := runner.RunOnce(ctx); !errors.Is(err, publish.ErrRetryable) {
		t.Fatalf("first RunOnce() error = %v, want retryable publish error", err)
	}
	afterFailure, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobStore.Get(ctx, "job_retry_publish")
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Status != generation.StatusValidating || job.Status != jobs.StatusQueued {
		t.Fatalf("after retry scheduling session=%s job=%s", afterFailure.Status, job.Status)
	}
	current = current.Add(time.Second)
	outcome, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.VersionID != "ver_retry" || len(provider.requests) != 2 {
		t.Fatalf("outcome=%#v provider calls=%d", outcome, len(provider.requests))
	}
	detail, err := publisher.Card(ctx, "user", "card_retry")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Versions) != 1 || detail.Versions[0].VersionID != "ver_retry" {
		t.Fatalf("versions = %#v", detail.Versions)
	}
}

func TestWorkerRetriesAfterObjectUploadWhenVersionInsertTemporarilyFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	current := time.Date(2026, 7, 15, 12, 30, 0, 0, time.UTC)
	service := generation.NewService(generation.NewMemoryRepository(), func() string { return "gen_version_retry" }, func() time.Time { return current })
	session, err := service.Create(ctx, "user", generation.CreateRequest{Prompt: "版本重试", Target: generation.TargetNative, Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_version_retry" })
	if _, err := jobStore.Enqueue(ctx, session.ID, current); err != nil {
		t.Fatal(err)
	}
	provider := &captureProvider{}
	objects := &countingObjectStore{MemoryObjectStore: publish.NewMemoryObjectStore()}
	versions := &temporaryFailureVersionRepository{MemoryVersionRepository: publish.NewMemoryVersionRepository()}
	seed := sha256.Sum256([]byte("version-retry-key"))
	publisher := publish.NewPublisher(artifact.NewBuilder("version-key", ed25519.NewKeyFromSeed(seed[:])), objects, versions)
	runner := worker.New(worker.Config{
		WorkerID: "worker-version-retry", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()), Publisher: publisher,
		NewCardID: func() string { return "card_version_retry" }, NewVersionID: func() string { return "ver_version_retry" },
		Now: func() time.Time { return current },
	})

	if _, err := runner.RunOnce(ctx); !errors.Is(err, publish.ErrRetryable) {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	current = current.Add(time.Second)
	if _, err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	detail, err := publisher.Card(ctx, "user", "card_version_retry")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Versions) != 1 || objects.puts != 2 || len(provider.requests) != 2 ||
		len(objects.keys) != 2 || objects.keys[0] != objects.keys[1] {
		t.Fatalf("versions=%d puts=%d keys=%#v providerCalls=%d", len(detail.Versions), objects.puts, objects.keys, len(provider.requests))
	}
}

func TestWorkerCancellationPropagatesToBuilderAndObjectUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		build  func(*stageCancellation) (*agent.CodingAgent, publish.ObjectStore)
	}{
		{
			name: "CodeCard builder", prompt: "做一个自由绘制画板",
			build: func(stage *stageCancellation) (*agent.CodingAgent, publish.ObjectStore) {
				provider := staticProvider{content: `{"files":{"src/card.tsx":"export default function Card(){return <canvas/>}"}}`}
				return agent.NewCodingAgent(
					provider, agent.NewNativeValidator(), agent.WithWebBuilder(blockingWebBuilder{stage: stage}),
				), publish.NewMemoryObjectStore()
			},
		},
		{
			name: "object upload", prompt: "做一个离线文本卡片",
			build: func(stage *stageCancellation) (*agent.CodingAgent, publish.ObjectStore) {
				return agent.NewCodingAgent(&captureProvider{}, agent.NewNativeValidator()), blockingObjectStore{stage: stage}
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			jobStore := jobs.NewMemoryStore(func() string { return "job_stage_cancel" })
			service := generation.NewService(
				generation.NewMemoryRepository(), func() string { return "gen_stage_cancel" }, time.Now,
				generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
			)
			session, err := service.Create(ctx, "user", generation.CreateRequest{
				Prompt: testCase.prompt, Target: generation.TargetAuto, Locale: "zh-CN",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
				t.Fatal(err)
			}
			stage := &stageCancellation{started: make(chan struct{}), cancelled: make(chan struct{})}
			codingAgent, objectStore := testCase.build(stage)
			seed := sha256.Sum256([]byte("stage-cancel-key"))
			versions := publish.NewMemoryVersionRepository()
			runner := worker.New(worker.Config{
				WorkerID: "worker-stage-cancel", Jobs: jobStore, Generations: service, Agent: codingAgent,
				Publisher: publish.NewPublisher(artifact.NewBuilder("stage-key", ed25519.NewKeyFromSeed(seed[:])), objectStore, versions),
				NewCardID: func() string { return "card_stage_cancel" }, NewVersionID: func() string { return "ver_stage_cancel" },
				Now: time.Now, LeaseDuration: 40 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond,
			})
			done := make(chan error, 1)
			go func() { _, runErr := runner.RunOnce(ctx); done <- runErr }()
			<-stage.started
			if _, err := service.Cancel(ctx, "user", session.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-stage.cancelled:
			case <-time.After(time.Second):
				t.Fatal("stage did not observe cancellation")
			}
			if err := <-done; !errors.Is(err, jobs.ErrConflict) {
				t.Fatalf("RunOnce() error = %v", err)
			}
			stored, err := service.Get(ctx, "user", session.ID)
			if err != nil {
				t.Fatal(err)
			}
			published, err := versions.ListByUser(ctx, "user")
			if err != nil || stored.Status != generation.StatusCancelled || len(published) != 0 {
				t.Fatalf("session=%s versions=%d error=%v", stored.Status, len(published), err)
			}
		})
	}
}

func TestWorkerPublishesSandboxedCodeCard(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	service := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return "gen_web" },
		func() time.Time { return now },
	)
	session, err := service.Create(
		context.Background(),
		"user",
		generation.CreateRequest{
			Prompt: "做一个自由绘制的离线画板",
			Target: generation.TargetAuto,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "user", session.ID); err != nil {
		t.Fatal(err)
	}
	jobStore := jobs.NewMemoryStore(func() string { return "job_web" })
	if _, err := jobStore.Enqueue(context.Background(), session.ID, now); err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("web-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publisher := publish.NewPublisher(
		artifact.NewBuilder("key", privateKey),
		publish.NewMemoryObjectStore(),
		publish.NewMemoryVersionRepository(),
	)
	codingAgent := agent.NewCodingAgent(
		staticProvider{content: `{"files":{"src/card.tsx":"export function Card(){return <canvas/>}"}}`},
		agent.NewNativeValidator(),
		agent.WithWebBuilder(staticWebBuilder{}),
	)
	runner := worker.New(worker.Config{
		WorkerID:     "worker",
		Jobs:         jobStore,
		Generations:  service,
		Agent:        codingAgent,
		Publisher:    publisher,
		NewCardID:    func() string { return "card_web" },
		NewVersionID: func() string { return "ver_web" },
		Now:          func() time.Time { return now },
	})

	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	card, err := publisher.Card(context.Background(), "user", "card_web")
	if err != nil {
		t.Fatal(err)
	}
	if card.Versions[0].Runtime != "web" {
		t.Fatalf("runtime = %q, want web", card.Versions[0].Runtime)
	}
	loaded, err := publisher.LoadVersionArtifact(context.Background(), "user", "card_web", "ver_web", 8*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := agent.ParseBaseArtifact(loaded.Archive, agent.BaseArtifactExpectation{
		CardID: "card_web", VersionID: "ver_web", KeyID: "key",
	}, map[string]ed25519.PublicKey{"key": privateKey.Public().(ed25519.PublicKey)})
	if err != nil {
		t.Fatalf("published CodeCard source is not reusable: %v", err)
	}
	if parsed.Sources["src/card.tsx"] == "" {
		t.Fatalf("published CodeCard sources = %#v", parsed.Sources)
	}
}

type staticProvider struct {
	content string
}

type blockingProvider struct{}

func (blockingProvider) Generate(ctx context.Context, _ modelprovider.Request) (modelprovider.Response, error) {
	<-ctx.Done()
	return modelprovider.Response{}, ctx.Err()
}

type leaseLosingStore struct {
	jobs.Store
}

type completeFailOnceStore struct {
	jobs.Store
	failed bool
}

func (store *completeFailOnceStore) Complete(
	ctx context.Context, jobID, workerID string, at time.Time,
) error {
	if !store.failed {
		store.failed = true
		return publish.ErrRetryable
	}
	return store.Store.Complete(ctx, jobID, workerID, at)
}

type cancellationProvider struct {
	started   chan struct{}
	cancelled chan struct{}
}

func (provider *cancellationProvider) Generate(
	ctx context.Context, _ modelprovider.Request,
) (modelprovider.Response, error) {
	close(provider.started)
	<-ctx.Done()
	close(provider.cancelled)
	return modelprovider.Response{}, ctx.Err()
}

func (*leaseLosingStore) ExtendLease(
	context.Context, string, string, time.Time, time.Duration,
) error {
	return jobs.ErrConflict
}

type captureProvider struct {
	requests []modelprovider.Request
}

type captureObjectStore struct {
	archive []byte
}

type temporaryFailureObjectStore struct {
	*publish.MemoryObjectStore
	failed bool
}

type countingObjectStore struct {
	*publish.MemoryObjectStore
	puts int
	keys []string
}

func (store *countingObjectStore) PutIfAbsent(ctx context.Context, key string, content []byte) error {
	store.puts++
	store.keys = append(store.keys, key)
	return store.MemoryObjectStore.PutIfAbsent(ctx, key, content)
}

type temporaryFailureVersionRepository struct {
	*publish.MemoryVersionRepository
	failed bool
}

func (repository *temporaryFailureVersionRepository) Create(
	ctx context.Context, version publish.CardVersion,
) (publish.CardVersion, error) {
	if !repository.failed {
		repository.failed = true
		return publish.CardVersion{}, publish.ErrRetryable
	}
	return repository.MemoryVersionRepository.Create(ctx, version)
}

type stageCancellation struct {
	started   chan struct{}
	cancelled chan struct{}
}

type blockingWebBuilder struct{ stage *stageCancellation }

func (builder blockingWebBuilder) Build(ctx context.Context, _ map[string]string) (map[string][]byte, error) {
	close(builder.stage.started)
	<-ctx.Done()
	close(builder.stage.cancelled)
	return nil, ctx.Err()
}

type blockingObjectStore struct{ stage *stageCancellation }

func (store blockingObjectStore) PutIfAbsent(ctx context.Context, _ string, _ []byte) error {
	close(store.stage.started)
	<-ctx.Done()
	close(store.stage.cancelled)
	return ctx.Err()
}

func (blockingObjectStore) SignedURL(context.Context, string, time.Duration) (string, time.Time, error) {
	return "", time.Time{}, publish.ErrNotFound
}

func (store *temporaryFailureObjectStore) PutIfAbsent(
	ctx context.Context, key string, content []byte,
) error {
	if !store.failed {
		store.failed = true
		return publish.ErrRetryable
	}
	return store.MemoryObjectStore.PutIfAbsent(ctx, key, content)
}

type manifestMetadata struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type staticWebBuilder struct{}

func (staticWebBuilder) Build(context.Context, map[string]string) (map[string][]byte, error) {
	return map[string][]byte{
		"index.html":    []byte(`<script src="/runtime/bootstrap.js"></script><canvas></canvas>`),
		"assets/app.js": []byte("const offline = true"),
	}, nil
}

func (provider staticProvider) Generate(context.Context, modelprovider.Request) (modelprovider.Response, error) {
	return modelprovider.Response{Content: provider.content}, nil
}

func (provider *captureProvider) Generate(
	_ context.Context,
	request modelprovider.Request,
) (modelprovider.Response, error) {
	provider.requests = append(provider.requests, request)
	return modelprovider.Response{
		Content: `{"schemaVersion":1,"initialState":{"text":"完成"},"root":{"id":"root","type":"Text","props":{"text":{"path":"state.text"}}}}`,
	}, nil
}

func (store *captureObjectStore) PutIfAbsent(_ context.Context, _ string, content []byte) error {
	store.archive = append([]byte(nil), content...)
	return nil
}

func (*captureObjectStore) SignedURL(
	context.Context,
	string,
	time.Duration,
) (string, time.Time, error) {
	return "memory://artifact", time.Now().UTC(), nil
}

func newWorkerForTest(
	now time.Time,
	service *generation.Service,
	jobStore jobs.Store,
	codingAgent *agent.CodingAgent,
	objectStore publish.ObjectStore,
) *worker.Worker {
	seed := sha256.Sum256([]byte("snapshot-worker-key"))
	return worker.New(worker.Config{
		WorkerID:    "worker-snapshot",
		Jobs:        jobStore,
		Generations: service,
		Agent:       codingAgent,
		Publisher: publish.NewPublisher(
			artifact.NewBuilder("snapshot-key", ed25519.NewKeyFromSeed(seed[:])),
			objectStore,
			publish.NewMemoryVersionRepository(),
		),
		NewCardID:    func() string { return "card_snapshot" },
		NewVersionID: func() string { return "ver_snapshot" },
		Now:          func() time.Time { return now },
	})
}

func decodeManifest(t *testing.T, archive []byte) manifestMetadata {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open artifact: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatalf("open manifest: %v", err)
		}
		content, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatalf("read manifest: %v", readErr)
		}
		if closeErr != nil {
			t.Fatalf("close manifest: %v", closeErr)
		}
		var manifest manifestMetadata
		if err := json.Unmarshal(content, &manifest); err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		return manifest
	}
	t.Fatal("manifest.json missing")
	return manifestMetadata{}
}
