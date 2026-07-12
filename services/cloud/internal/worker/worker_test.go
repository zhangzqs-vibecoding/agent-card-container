package worker_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/worker"
)

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
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())
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
	runner := worker.New(worker.Config{
		WorkerID:     "worker",
		Jobs:         jobStore,
		Generations:  service,
		Agent:        agent.NewCodingAgent(staticProvider{content: "{}"}, agent.NewNativeValidator()),
		Publisher:    publish.NewPublisher(artifact.NewBuilder("key", ed25519.NewKeyFromSeed(seed[:])), publish.NewMemoryObjectStore(), publish.NewMemoryVersionRepository()),
		NewCardID:    func() string { return "card_fail" },
		NewVersionID: func() string { return "ver_fail" },
		Now:          func() time.Time { return now },
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
