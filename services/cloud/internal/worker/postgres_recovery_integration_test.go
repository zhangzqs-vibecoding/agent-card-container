package worker_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
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

func TestProductionWorkerPublishesAndRecoversSandboxedCodeCard(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	s3Endpoint := os.Getenv("AGENTCARD_S3_TEST_ENDPOINT")
	sandboxImage := os.Getenv("AGENTCARD_SANDBOX_TEST_IMAGE")
	if dsn == "" || s3Endpoint == "" || sandboxImage == "" {
		t.Skip("production CodeCard persistence environment is not configured")
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
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	versions := publish.NewPostgresVersionRepository(database)
	publisher := publish.NewPublisher(artifact.NewBuilder("p1c-key", privateKey), objects, versions)
	jobStore := jobs.NewPostgresStore(database, func() string { return "job_p1c_codecard" })
	createdAt := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	service := generation.NewService(
		generation.NewPostgresRepository(database), func() string { return "gen_p1c_codecard" }, func() time.Time { return createdAt },
		generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
		generation.WithAtomicJobID(func() string { return "job_p1c_codecard" }),
	)
	session, err := service.Create(ctx, "user", generation.CreateRequest{
		Prompt: "生成一个纯离线计数器", Target: generation.TargetWeb, Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(ctx, "user", session.ID); err != nil {
		t.Fatal(err)
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	provider := &codeCardIntegrationProvider{}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agent.WithWebBuilder(agent.NewTemplateWebBuilder(
			filepath.Join(repository, "tooling", "codecard-template"),
			sandbox.NewDockerBuilder(sandbox.DockerConfig{Image: sandboxImage}, sandbox.ExecRunner{}),
		)),
	)
	firstStore := &completeFailOnceStore{Store: jobStore}
	first := worker.New(worker.Config{
		WorkerID: "worker-codecard-first", Jobs: firstStore, Generations: service,
		Agent: codingAgent, Publisher: publisher,
		NewCardID: func() string { return "card_p1c_codecard" }, NewVersionID: func() string { return "ver_p1c_codecard" },
		Now: func() time.Time { return createdAt },
	})
	if _, err := first.RunOnce(ctx); !errors.Is(err, publish.ErrRetryable) {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("first provider calls = %d, want 1", provider.calls)
	}
	version, err := versions.Find(ctx, "user", "card_p1c_codecard", "ver_p1c_codecard")
	if err != nil {
		t.Fatal(err)
	}
	download, err := publisher.Download(ctx, "user", version.CardID, version.VersionID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verifyPersistedCodeCard(t, download, version, "p1c-key", publicKey)

	recoveryProvider := &codeCardIntegrationProvider{}
	recoveredAt := createdAt.Add(2 * time.Minute)
	second := worker.New(worker.Config{
		WorkerID: "worker-codecard-recovery", Jobs: jobStore, Generations: service,
		Agent: agent.NewCodingAgent(recoveryProvider, agent.NewNativeValidator()), Publisher: publisher,
		NewCardID: func() string { return "card_other" }, NewVersionID: func() string { return "ver_other" },
		Now: func() time.Time { return recoveredAt },
	})
	if _, err := second.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if recoveryProvider.calls != 0 {
		t.Fatalf("recovery provider calls = %d, want 0", recoveryProvider.calls)
	}
	ready, err := service.Get(ctx, "user", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := jobStore.Get(ctx, "job_p1c_codecard")
	if err != nil {
		t.Fatal(err)
	}
	var versionCount int
	if err := database.QueryRow(`SELECT count(*) FROM card_versions WHERE card_id = $1`, version.CardID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if ready.Status != generation.StatusReady || completed.Status != jobs.StatusCompleted || versionCount != 1 {
		t.Fatalf("ready=%s job=%s versionCount=%d", ready.Status, completed.Status, versionCount)
	}
}

type codeCardIntegrationProvider struct {
	calls int
}

func verifyPersistedCodeCard(
	t *testing.T,
	download publish.Download,
	version publish.CardVersion,
	keyID string,
	publicKey ed25519.PublicKey,
) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(download.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("artifact download status = %d", response.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(archive) == 0 || len(archive) > 8*1024*1024 {
		t.Fatal("artifact archive size is invalid")
	}
	digest := sha256.Sum256(archive)
	if encoded := hex.EncodeToString(digest[:]); encoded != download.SHA256 || encoded != version.ArtifactSHA256 {
		t.Fatalf("artifact digest = %s", encoded)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string][]byte, len(reader.File))
	var expanded uint64
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || file.Mode()&os.ModeSymlink != 0 || entries[file.Name] != nil {
			t.Fatalf("unsafe artifact entry %q", file.Name)
		}
		expanded += file.UncompressedSize64
		if expanded > 32*1024*1024 || file.UncompressedSize64 > 8*1024*1024 {
			t.Fatal("artifact expanded size is invalid")
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil || uint64(len(content)) != file.UncompressedSize64 {
			t.Fatalf("read artifact entry %q", file.Name)
		}
		entries[file.Name] = content
	}
	for _, required := range []string{
		"manifest.json",
		"payload/web/index.html",
		"payload/web/dependency-policy.json",
		"reports/validation.json",
	} {
		if len(entries[required]) == 0 {
			t.Fatalf("artifact is missing %s", required)
		}
	}
	var manifest map[string]any
	decoder := json.NewDecoder(bytes.NewReader(entries["manifest.json"]))
	decoder.UseNumber()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	signatureText, signatureOK := manifest["signature"].(string)
	manifestKeyID, keyOK := manifest["keyId"].(string)
	if !signatureOK || !keyOK || manifestKeyID != keyID || download.KeyID != keyID {
		t.Fatal("artifact signing metadata is invalid")
	}
	delete(manifest, "signature")
	signature, err := base64.RawURLEncoding.DecodeString(signatureText)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := artifact.CanonicalJSON(manifest)
	if err != nil || !ed25519.Verify(publicKey, canonical, signature) {
		t.Fatal("artifact signature is invalid")
	}
	delete(manifest, "keyId")
	definitionBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := contracts.DecodeCardDefinition(bytes.NewReader(definitionBytes))
	if err != nil {
		t.Fatal(err)
	}
	if definition.CardID != version.CardID ||
		definition.VersionID != version.VersionID ||
		definition.Runtime != contracts.CardRuntimeWeb ||
		definition.Entrypoint != "payload/web/index.html" ||
		definition.CatalogVersion != "" ||
		definition.NetworkPolicy.Mode != "none" ||
		len(definition.NetworkPolicy.Domains) != 0 ||
		!slices.Equal(definition.Capabilities, []string{"storage", "window.manageSelf"}) ||
		len(definition.Files) != len(entries)-1 {
		t.Fatalf(
			"artifact manifest metadata is inconsistent: runtime=%q entrypoint=%q catalog=%q capabilities=%d network=%q domains=%d files=%d entries=%d",
			definition.Runtime,
			definition.Entrypoint,
			definition.CatalogVersion,
			len(definition.Capabilities),
			definition.NetworkPolicy.Mode,
			len(definition.NetworkPolicy.Domains),
			len(definition.Files),
			len(entries),
		)
	}
	seen := make(map[string]bool, len(definition.Files))
	for _, declared := range definition.Files {
		content, exists := entries[declared.Path]
		if !exists || seen[declared.Path] || declared.Size != int64(len(content)) {
			t.Fatalf("artifact manifest file %q is inconsistent", declared.Path)
		}
		seen[declared.Path] = true
		fileDigest := sha256.Sum256(content)
		if declared.SHA256 != hex.EncodeToString(fileDigest[:]) {
			t.Fatalf("artifact manifest hash for %q is invalid", declared.Path)
		}
	}
}

func (provider *codeCardIntegrationProvider) Generate(
	context.Context,
	modelprovider.Request,
) (modelprovider.Response, error) {
	provider.calls++
	return modelprovider.Response{Content: `{"files":{"src/card.tsx":"import { useState } from 'preact/hooks'; export function Card(){const [count,setCount]=useState(0);return <button onClick={()=>setCount(count+1)}>Count {count}</button>}","src/card.css":"button{color:CanvasText;background:Canvas}"}}`}, nil
}
