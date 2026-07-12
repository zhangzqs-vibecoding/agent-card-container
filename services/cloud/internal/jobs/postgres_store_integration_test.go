package jobs_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
)

func TestPostgresStoreLeasesAndCompletesJobsAcrossWorkers(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("AGENTCARD_POSTGRES_TEST_DSN is not configured")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	resetJobsPostgres(t, database)
	store := jobs.NewPostgresStore(database, func() string { return "job_postgres" })
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	if _, err := database.Exec(
		`INSERT INTO generation_sessions
		 (id,user_id,prompt,target,locale,status,summary_json,created_at,updated_at)
		 VALUES ('gen_job','owner','card','auto','zh-CN','queued','{}',$1,$1)`,
		now,
	); err != nil {
		t.Fatal(err)
	}

	first, err := store.Enqueue(context.Background(), "gen_job", now)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.Enqueue(context.Background(), "gen_job", now)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != first.ID {
		t.Fatalf("repeated job ID = %q, want %q", repeated.ID, first.ID)
	}
	claimed, err := store.Claim(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != jobs.StatusRunning || claimed.Attempts != 1 {
		t.Fatalf("claimed job = %#v", claimed)
	}
	reclaimed, err := store.Claim(context.Background(), "worker-b", now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.LeaseOwner != "worker-b" || reclaimed.Attempts != 2 {
		t.Fatalf("reclaimed job = %#v", reclaimed)
	}
	if err := store.Complete(context.Background(), reclaimed.ID, "worker-a", now.Add(3*time.Minute)); err != jobs.ErrConflict {
		t.Fatalf("wrong owner completion error = %v", err)
	}
	if err := store.Complete(context.Background(), reclaimed.ID, "worker-b", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Get(context.Background(), reclaimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != jobs.StatusCompleted || !completed.LeaseUntil.IsZero() {
		t.Fatalf("completed job = %#v", completed)
	}
}

func resetJobsPostgres(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_initial.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(source)); err != nil {
		t.Fatal(err)
	}
}
