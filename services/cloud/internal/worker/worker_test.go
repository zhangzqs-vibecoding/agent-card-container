package worker_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/worker"
)

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
	publisher := publish.NewPublisher(
		artifact.NewBuilder("key", ed25519.NewKeyFromSeed(seed[:])),
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
}

type staticProvider struct {
	content string
}

type captureProvider struct {
	requests []modelprovider.Request
}

type captureObjectStore struct {
	archive []byte
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
