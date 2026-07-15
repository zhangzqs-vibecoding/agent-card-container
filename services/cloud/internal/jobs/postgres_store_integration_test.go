package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/migrations"
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
		 VALUES ('gen_job','owner','card','auto','zh-CN','awaiting_confirmation','{}',$1,$1)`,
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
	completionAt := now.Add(150 * time.Second)
	if err := store.Complete(context.Background(), reclaimed.ID, "worker-a", completionAt); err != jobs.ErrConflict {
		t.Fatalf("wrong owner completion error = %v", err)
	}
	if err := store.Complete(context.Background(), reclaimed.ID, "worker-b", completionAt); err != nil {
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

func TestPostgresStorePersistsLeasePublicationAndRetrySchedule(t *testing.T) {
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
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if _, err := database.Exec(
		`INSERT INTO generation_sessions
		 (id,user_id,prompt,target,locale,status,summary_json,created_at,updated_at)
		 VALUES ('gen_reliable','owner','card','auto','zh-CN','awaiting_confirmation','{}',$1,$1)`,
		now,
	); err != nil {
		t.Fatal(err)
	}
	store := jobs.NewPostgresStore(database, func() string { return "job_reliable" })
	job, err := store.Enqueue(context.Background(), "gen_reliable", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker-a", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.ExtendLease(context.Background(), job.ID, "worker-a", now.Add(30*time.Second), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.ExtendLease(context.Background(), job.ID, "worker-b", now.Add(40*time.Second), time.Minute); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("wrong owner ExtendLease() error = %v", err)
	}
	first, err := store.ReservePublication(
		context.Background(), job.ID, "worker-a", "card_stable", "ver_stable", now.Add(40*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.ReservePublication(
		context.Background(), job.ID, "worker-a", "card_other", "ver_other", now.Add(41*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if first != repeated || first.CardID != "card_stable" || first.VersionID != "ver_stable" {
		t.Fatalf("publications = %#v, %#v", first, repeated)
	}
	availableAt := now.Add(2 * time.Minute)
	if err := store.Retry(context.Background(), job.ID, "worker-a", now.Add(50*time.Second), availableAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), "worker-b", availableAt.Add(-time.Nanosecond), time.Minute); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatalf("early Claim() error = %v", err)
	}
	retried, err := store.Claim(context.Background(), "worker-b", availableAt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Attempts != 2 || retried.CardID != "card_stable" || retried.VersionID != "ver_stable" {
		t.Fatalf("retried job = %#v", retried)
	}
}

func resetJobsPostgres(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Apply(context.Background(), database); err != nil {
		t.Fatal(err)
	}
}
