package worker_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/worker"
	"github.com/zzq/agent-card-container/services/cloud/migrations"
)

func TestProductionWorkerRecoversPublishedVersionAfterLeaseExpiry(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	s3Endpoint := os.Getenv("AGENTCARD_S3_TEST_ENDPOINT")
	if dsn == "" || s3Endpoint == "" {
		t.Skip("production persistence test environment is not configured")
	}
	ctx := context.Background()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Apply(ctx, database); err != nil {
		t.Fatal(err)
	}
	objects, err := publish.NewS3ObjectStore(ctx, publish.S3Config{
		Endpoint: s3Endpoint, AccessKey: os.Getenv("AGENTCARD_S3_TEST_ACCESS_KEY"),
		SecretKey: os.Getenv("AGENTCARD_S3_TEST_SECRET_KEY"), Bucket: os.Getenv("AGENTCARD_S3_TEST_BUCKET"),
		Region: "us-east-1", Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	versions := publish.NewPostgresVersionRepository(database)
	publisher := publish.NewPublisher(artifact.NewBuilder("p1b-key", privateKey), objects, versions)
	jobStore := jobs.NewPostgresStore(database, func() string { return "job_p1b_recovery" })
	firstAttemptAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	service := generation.NewService(
		generation.NewPostgresRepository(database), func() string { return "gen_p1b_recovery" }, func() time.Time { return firstAttemptAt },
		generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
		generation.WithAtomicJobID(func() string { return "job_p1b_recovery" }),
	)
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "持久化恢复", Target: generation.TargetNative, Locale: "zh-CN",
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
	job, err := jobStore.Claim(ctx, "worker-crashed", firstAttemptAt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := jobStore.ReservePublication(
		ctx, job.ID, "worker-crashed", "card_p1b_recovery", "ver_p1b_recovery", firstAttemptAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	definition := contracts.CardDefinition{
		FormatVersion: 1, MinHostVersion: "1.0.0", CardID: publication.CardID, VersionID: publication.VersionID,
		DisplayVersion: "1.0.0", Runtime: contracts.CardRuntimeNative, StateSchemaVersion: 1,
		Title: "持久化恢复", Entrypoint: "payload/native.json", CatalogVersion: "1",
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
	retryAt := firstAttemptAt.Add(2 * time.Minute)
	runner := worker.New(worker.Config{
		WorkerID: "worker-recovery", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(provider, agent.NewNativeValidator()), Publisher: publisher,
		NewCardID: func() string { return "card_other" }, NewVersionID: func() string { return "ver_other" },
		Now: func() time.Time { return retryAt },
	})
	if _, err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(provider.requests))
	}
	ready, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := jobStore.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var versionCount int
	if err := database.QueryRow(
		`SELECT count(*) FROM card_versions WHERE card_id = $1`, publication.CardID,
	).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if ready.Status != generation.StatusReady || completed.Status != jobs.StatusCompleted || versionCount != 1 {
		t.Fatalf("ready=%s job=%s versionCount=%d", ready.Status, completed.Status, versionCount)
	}
}
